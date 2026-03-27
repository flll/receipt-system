package main

import (
	"cloud.google.com/go/storage"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
	"google.golang.org/api/option"
)

// ✷ アプリケーション設定
type Config struct {
	Protocol         string   `json:"protocol"`
	PrinterIP        string   `json:"printerIP"`
	DevID            string   `json:"devid"`
	Phone            string   `json:"phone"`
	Address          string   `json:"address"`
	Amount           string   `json:"amount"`
	ReceiptURL       string   `json:"receiptURL"`
	IssuerName       string   `json:"issuerName"`
	ProjectID        string   `json:"projectId"`
	BucketName       string   `json:"bucketName"`
	FirebaseAPIKey   string   `json:"firebaseApiKey"`
	FirebaseAuthDom  string   `json:"firebaseAuthDomain"`
	FirebaseProjectID string  `json:"firebaseProjectId"`
	AllowedEmails    []string `json:"allowedEmails"`
}

// ✷ サーバーの中核
type Server struct {
	config     *Config
	configMu   sync.RWMutex
	authClient *auth.Client
	gcsBucket  *storage.BucketHandle
	mux        *http.ServeMux
	indexHTML   []byte
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv, err := newServer()
	if err != nil {
		log.Fatalf("サーバー初期化エラー: %v", err)
	}

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      srv.middleware(srv.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ✷ Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("Server running at http://localhost:%s", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("サーバー起動エラー: %v", err)
		}
	}()

	<-stop
	log.Println("シャットダウン中...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("シャットダウンエラー: %v", err)
	}
	log.Println("サーバー停止完了")
}

func newServer()(* Server, error) {
	s := &Server{}

	// ✷ 設定読み込み
	if err := s.loadConfig(); err != nil {
		return nil, fmt.Errorf("設定読み込みエラー: %w", err)
	}

	ctx := context.Background()

	// ✷ Firebase Admin SDK 初期化
	var firebaseApp *firebase.App
	saPath := "./config/firebase-service-account-key.json"
	if _, err := os.Stat(saPath); err == nil {
		opt := option.WithCredentialsFile(saPath)
		firebaseApp, err = firebase.NewApp(ctx, nil, opt)
		if err != nil {
			return nil, fmt.Errorf("Firebase初期化エラー: %w", err)
		}
	} else {
		var err error
		firebaseApp, err = firebase.NewApp(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("Firebase初期化エラー: %w", err)
		}
	}

	authClient, err := firebaseApp.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("Firebase Auth初期化エラー: %w", err)
	}
	s.authClient = authClient
	log.Println("Firebase Admin SDK初期化完了")

	// ✷ GCS 初期化
	var storageClient *storage.Client
	if _, err := os.Stat(saPath); err == nil {
		storageClient, err = storage.NewClient(ctx, option.WithCredentialsFile(saPath))
	} else {
		storageClient, err = storage.NewClient(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("GCS初期化エラー: %w", err)
	}
	s.gcsBucket = storageClient.Bucket(s.getConfig().BucketName)

	// ✷ index.html 読み込み
	indexData, err := os.ReadFile("./index.html")
	if err != nil {
		return nil, fmt.Errorf("index.html読み込みエラー: %w", err)
	}
	s.indexHTML = indexData

	// ✷ ルーティング設定
	s.setupRoutes()

	return s, nil
}

func (s *Server) loadConfig() error {
	data, err := os.ReadFile("./config/config.json")
	if err != nil {
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	s.configMu.Lock()
	s.config = &cfg
	s.configMu.Unlock()
	return nil
}

func (s *Server) getConfig() Config {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return *s.config
}

// ✷ CSRF トークン生成
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ✷ allowedEmails チェック
func (s *Server) isEmailAllowed(email string) bool {
	cfg := s.getConfig()
	for _, e := range cfg.AllowedEmails {
		if e == email {
			return true
		}
	}
	return false
}

// ✷ クライアント IP 取得（Cloudflare 対応）
func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("Cf-Connecting-Ip"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	return r.RemoteAddr
}

// ✷ JSON レスポンス送信
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ✷ Cookie からセッション Cookie を取得
func getSessionCookie(r *http.Request) string {
	c, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return c.Value
}

func getCookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

// ✷ Cloud Run では K_SERVICE が自動セットされるためフォールバック判定に使用
func isProduction() bool {
	if os.Getenv("ENV") == "production" || os.Getenv("GO_ENV") == "production" {
		return true
	}
	return os.Getenv("K_SERVICE") != ""
}

// ✷ ルーティング設定
func (s *Server) setupRoutes() {
	mux := http.NewServeMux()

	// ✷ 静的ファイル
	mux.Handle("GET /js/", http.StripPrefix("/js/", http.FileServer(http.Dir("./js"))))
	mux.Handle("GET /editor/", http.StripPrefix("/editor/", http.FileServer(http.Dir("./editor"))))

	// ✷ 公開 API（認証不要）
	mux.HandleFunc("GET /api/firebase-config", s.handleFirebaseConfig)
	mux.HandleFunc("GET /api/csrf-token", s.handleCSRFToken)
	mux.HandleFunc("GET /api/receipt/{uuid}", s.handleGetReceipt)
	mux.HandleFunc("GET /api/qr/{uuid}", s.handleQRCode)
	mux.HandleFunc("POST /sessionLogin", s.handleSessionLogin)
	mux.HandleFunc("GET /logout", s.handleLogout)
	mux.HandleFunc("GET /_ah/warmup", s.handleWarmup)

	// ✷ 認証付き API
	mux.HandleFunc("POST /api/save-receipt", s.handleSaveReceipt)
	mux.HandleFunc("GET /api/generate-uuid", s.handleGenerateUUID)
	mux.HandleFunc("GET /config/config.json", s.handleConfigJSON)

	// ✷ ページ配信
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("GET /receipt", s.handleReceiptPage)
	mux.HandleFunc("GET /", s.handleIndex)

	// ✷ ミドルウェアチェーンを適用
	s.mux = mux
}

// ✷ ミドルウェア: リクエストログ + CORS + セキュリティヘッダ
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ✷ リクエストログ
		log.Printf("%s %s %s", r.Method, getClientIP(r), r.URL.Path)

		// ✷ CORS
		cfg := s.getConfig()
		allowedOrigins := map[string]bool{
			"https://receipt-printer.lll.fish":        true,
			"http://localhost:8080":                   true,
			"https://localhost:8080":                  true,
			"https://" + cfg.PrinterIP:                true,
			"http://" + cfg.PrinterIP:                 true,
		}
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, SOAPAction, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Cross-Origin-Opener-Policy", "unsafe-none")
		w.Header().Set("Cross-Origin-Embedder-Policy", "unsafe-none")

		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(204)
			return
		}

		// ✷ セキュリティヘッダ（Helmet 相当）
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("X-Powered-By", "")
		if isProduction() {
			w.Header().Set("Strict-Transport-Security", "max-age=15552000; includeSubDomains; preload")
		}

		// ✷ CSP
		printerIP := cfg.PrinterIP
		csp := strings.Join([]string{
			"default-src 'self'",
			"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://www.gstatic.com https://*.googleapis.com https://apis.google.com https://accounts.google.com https://*.cloudflareinsights.com",
			"style-src 'self' 'unsafe-inline' https://www.gstatic.com https://*.googleapis.com https://fonts.googleapis.com",
			"img-src 'self' data: https: blob:",
			"connect-src 'self' https://identitytoolkit.googleapis.com https://securetoken.googleapis.com https://*.googleapis.com https://www.googleapis.com https://apis.google.com https://accounts.google.com https://*.lll.fish https://*.cloudflareinsights.com https://kitchen-printer.lll.fish https://www.gstatic.com https://" + printerIP,
			"frame-src 'self' https://accounts.google.com https://*.firebaseapp.com https://*.googleapis.com",
			"form-action 'self' https://accounts.google.com",
			"object-src 'none'",
			"worker-src 'self' blob:",
			"child-src 'self' blob: https://accounts.google.com",
			"base-uri 'self'",
			"font-src 'self' https://fonts.gstatic.com",
		}, "; ")
		w.Header().Set("Content-Security-Policy", csp)

		next.ServeHTTP(w, r)
	})
}

// ========== ページハンドラ ==========

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./views/login.html")
}

func (s *Server) handleReceiptPage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./views/receipt.html")
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// ✷ セッション検証 + allowedEmails チェック
	sessionCookie := getSessionCookie(r)
	if sessionCookie == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	token, err := s.authClient.VerifySessionCookieAndCheckRevoked(r.Context(), sessionCookie)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	email, _ := token.Claims["email"].(string)
	if !s.isEmailAllowed(email) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// ✷ CSRF トークン埋め込み
	csrfToken, err := generateCSRFToken()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "サーバーエラー"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf-token",
		Value:    csrfToken,
		HttpOnly: true,
		Secure:   isProduction(),
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	// ✷ no-cache ヘッダ
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	html := strings.Replace(string(s.indexHTML), `content=""`, fmt.Sprintf(`content="%s"`, csrfToken), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// ========== API ハンドラ ==========

func (s *Server) handleFirebaseConfig(w http.ResponseWriter, _ *http.Request) {
	cfg := s.getConfig()
	writeJSON(w, 200, map[string]string{
		"apiKey":     cfg.FirebaseAPIKey,
		"authDomain": cfg.FirebaseAuthDom,
		"projectId":  cfg.FirebaseProjectID,
	})
}

func (s *Server) handleCSRFToken(w http.ResponseWriter, _ *http.Request) {
	token, err := generateCSRFToken()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "トークン生成エラー"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf-token",
		Value:    token,
		HttpOnly: true,
		Secure:   isProduction(),
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	writeJSON(w, 200, map[string]string{"csrfToken": token})
}

func (s *Server) handleSessionLogin(w http.ResponseWriter, r *http.Request) {
	// ✷ ログイン CSRF 防止: Origin ヘッダーで同一オリジンを検証
	origin := r.Header.Get("Origin")
	if origin != "" {
		cfg := s.getConfig()
		allowedOrigins := map[string]bool{
			"https://receipt-printer.lll.fish": true,
			"http://localhost:8080":            true,
			"https://localhost:8080":           true,
			"https://" + cfg.PrinterIP:         true,
			"http://" + cfg.PrinterIP:          true,
		}
		if !allowedOrigins[origin] {
			log.Printf("sessionLogin Origin拒否: %s", origin)
			writeJSON(w, 403, map[string]string{"error": "不正なリクエスト元です"})
			return
		}
	}

	var body struct {
		IDToken string `json:"idToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "リクエスト不正"})
		return
	}

	token, err := s.authClient.VerifyIDToken(r.Context(), body.IDToken)
	if err != nil {
		log.Printf("sessionLoginエラー: %v", err)
		writeJSON(w, 401, map[string]string{"error": "無効な認証トークンです"})
		return
	}

	email, _ := token.Claims["email"].(string)
	if !s.isEmailAllowed(email) {
		s.authClient.RevokeRefreshTokens(r.Context(), token.UID)
		writeJSON(w, 403, map[string]any{
			"error":  "アクセス権限がありません",
			"action": "logout",
		})
		return
	}

	// ✷ セッション Cookie 発行（5日間）
	expiresIn := 5 * 24 * time.Hour
	cookie, err := s.authClient.SessionCookie(r.Context(), body.IDToken, expiresIn)
	if err != nil {
		log.Printf("セッションCookie作成エラー: %v", err)
		writeJSON(w, 500, map[string]string{"error": "セッション作成に失敗しました"})
		return
	}

	sameSite := http.SameSiteLaxMode
	if isProduction() {
		sameSite = http.SameSiteStrictMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    cookie,
		MaxAge:   int(expiresIn.Seconds()),
		HttpOnly: true,
		Secure:   isProduction(),
		SameSite: sameSite,
		Path:     "/",
	})

	writeJSON(w, 200, map[string]string{"status": "success"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionCookie := getSessionCookie(r)

	// ✷ Cookie 削除
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isProduction(),
		Path:     "/",
	})

	if sessionCookie != "" {
		token, err := s.authClient.VerifySessionCookie(r.Context(), sessionCookie)
		if err == nil {
			if err := s.authClient.RevokeRefreshTokens(r.Context(), token.UID); err != nil {
				log.Printf("セッション無効化エラー: %v", err)
			}
		}
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleGetReceipt(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.PathValue("uuid")

	// ✷ パストラバーサル防止: UUID 形式のバリデーション
	if _, err := uuid.Parse(uuidStr); err != nil {
		writeJSON(w, 400, map[string]any{
			"success": false,
			"error":   "無効なUUID形式です",
		})
		return
	}

	ctx := r.Context()
	obj := s.gcsBucket.Object(fmt.Sprintf("receipts/%s.json", uuidStr))
	reader, err := obj.NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			log.Printf("ファイルが存在しません: receipts/%s.json", uuidStr)
			writeJSON(w, 404, map[string]any{
				"success": false,
				"error":   "領収書が見つかりません",
			})
			return
		}
		log.Printf("Receipt fetch error: %v", err)
		writeJSON(w, 500, map[string]any{
			"success": false,
			"error":   "サーバーエラーが発生しました",
		})
		return
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("Receipt read error: %v", err)
		writeJSON(w, 500, map[string]any{
			"success": false,
			"error":   "サーバーエラーが発生しました",
		})
		return
	}

	log.Println("領収書データを正常に取得しました")
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// ✷ QRコード画像を生成（UUID専用、踏み台防止）
func (s *Server) handleQRCode(w http.ResponseWriter, r *http.Request) {
	uuidStr := r.PathValue("uuid")

	if _, err := uuid.Parse(uuidStr); err != nil {
		http.Error(w, "無効なUUID形式です", http.StatusBadRequest)
		return
	}

	s.configMu.RLock()
	receiptURL := s.config.ReceiptURL
	s.configMu.RUnlock()

	qrData := receiptURL + uuidStr
	png, err := qrcode.Encode(qrData, qrcode.High, 150)
	if err != nil {
		log.Printf("QRコード生成エラー: %v", err)
		http.Error(w, "QRコード生成に失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(png)
}

func (s *Server) handleSaveReceipt(w http.ResponseWriter, r *http.Request) {
	// ✷ CSRF 検証
	if !s.validateCSRF(r) {
		writeJSON(w, 403, map[string]string{"error": "CSRF検証に失敗しました"})
		return
	}

	// ✷ Bearer トークン認証
	email, err := s.verifyBearerToken(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "認証が必要です"})
		return
	}
	if !s.isEmailAllowed(email) {
		writeJSON(w, 403, map[string]string{"error": "このメールアドレスには操作権限がありません。config.jsonを確認してください。"})
		return
	}

	var body struct {
		UUID       string `json:"uuid"`
		Amount     string `json:"amount"`
		Datetime   string `json:"datetime"`
		Phone      string `json:"phone"`
		Address    string `json:"address"`
		IssuerName string `json:"issuerName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"success": false, "error": "リクエスト不正"})
		return
	}

	// ✷ GCS 任意パス書き込み防止: UUID 形式のバリデーション
	if _, err := uuid.Parse(body.UUID); err != nil {
		writeJSON(w, 400, map[string]any{"success": false, "error": "無効なUUID形式です"})
		return
	}

	receiptData, _ := json.MarshalIndent(body, "", "  ")
	ctx := r.Context()
	obj := s.gcsBucket.Object(fmt.Sprintf("receipts/%s.json", body.UUID))
	writer := obj.NewWriter(ctx)
	writer.ContentType = "application/json"
	writer.CacheControl = "public, max-age=31536000"

	if _, err := writer.Write(receiptData); err != nil {
		log.Printf("Receipt save error: %v", err)
		writeJSON(w, 500, map[string]any{"success": false, "error": err.Error()})
		return
	}
	if err := writer.Close(); err != nil {
		log.Printf("Receipt save error: %v", err)
		writeJSON(w, 500, map[string]any{"success": false, "error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{"success": true})
}

func (s *Server) handleGenerateUUID(w http.ResponseWriter, r *http.Request) {
	// ✷ セッション認証
	if !s.verifySession(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		log.Printf("UUID生成エラー: %v", err)
		writeJSON(w, 500, map[string]string{"error": "UUIDの生成に失敗しました"})
		return
	}
	writeJSON(w, 200, map[string]string{"uuid": id.String()})
}

func (s *Server) handleConfigJSON(w http.ResponseWriter, r *http.Request) {
	// ✷ セッション + allowedEmails チェック
	sessionCookie := getSessionCookie(r)
	if sessionCookie == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	token, err := s.authClient.VerifySessionCookieAndCheckRevoked(r.Context(), sessionCookie)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	email, _ := token.Claims["email"].(string)
	if !s.isEmailAllowed(email) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// ✷ フロントエンドに必要なフィールドのみ返却（allowedEmails 等の機密情報を除外）
	cfg := s.getConfig()
	writeJSON(w, 200, map[string]string{
		"protocol":   cfg.Protocol,
		"printerIP":  cfg.PrinterIP,
		"devid":      cfg.DevID,
		"phone":      cfg.Phone,
		"address":    cfg.Address,
		"amount":     cfg.Amount,
		"receiptURL": cfg.ReceiptURL,
		"issuerName": cfg.IssuerName,
	})
}

func (s *Server) handleWarmup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, err := s.gcsBucket.Attrs(ctx)
	if err != nil {
		log.Printf("ウォームアップエラー: %v", err)
		http.Error(w, "ウォームアップに失敗しました", 500)
		return
	}
	log.Println("ウォームアップが完了しました")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

// ========== 認証ヘルパー ==========

// ✷ Bearer トークンからメールアドレスを取得
func (s *Server) verifyBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", fmt.Errorf("Bearer token not found")
	}
	idToken := strings.TrimPrefix(authHeader, "Bearer ")

	token, err := s.authClient.VerifyIDToken(r.Context(), idToken)
	if err != nil {
		return "", err
	}
	email, _ := token.Claims["email"].(string)
	return email, nil
}

// ✷ セッション Cookie の簡易検証
func (s *Server) verifySession(r *http.Request) bool {
	sessionCookie := getSessionCookie(r)
	if sessionCookie == "" {
		return false
	}
	token, err := s.authClient.VerifySessionCookieAndCheckRevoked(r.Context(), sessionCookie)
	if err != nil {
		return false
	}
	email, _ := token.Claims["email"].(string)
	return s.isEmailAllowed(email)
}

// ✷ CSRF 検証（プリンタ IP からの直接リクエストのみスキップ）
func (s *Server) validateCSRF(r *http.Request) bool {
	cfg := s.getConfig()

	// ✷ プリンタからの直接リクエストは Host ヘッダのみで判定
	// X-Forwarded-Host や Origin は攻撃者が偽装可能なため使用しない
	if r.Host == cfg.PrinterIP {
		return true
	}

	csrfHeader := r.Header.Get("X-CSRF-Token")
	csrfCookie := getCookie(r, "csrf-token")

	if csrfHeader == "" || csrfCookie == "" || csrfHeader != csrfCookie {
		log.Printf("CSRF検証失敗: path=%s clientIP=%s origin=%s", r.URL.Path, getClientIP(r), r.Header.Get("Origin"))
		return false
	}
	return true
}
