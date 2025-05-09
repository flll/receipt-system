import express from 'express';
import path from 'path';
import { Storage } from '@google-cloud/storage';
import { fileURLToPath } from 'url';
import { dirname } from 'path';
import { v7 as uuidv7 } from 'uuid';
import { readFile } from 'fs/promises';
import { watch, existsSync, readFileSync } from 'fs';
import admin from 'firebase-admin';
import cookieParser from 'cookie-parser';
import session from 'express-session';
import helmet from 'helmet';
import crypto from 'crypto';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const DEFAULT_PORT = 8080;
const PORT = process.env.PORT ? parseInt(process.env.PORT, 10) : DEFAULT_PORT;

let storage;
let firebaseApp;
let config;

function getConfig() {
    if (!config) {
        config = JSON.parse(readFileSync('./config/config.json', 'utf8'));
    }
    return config;
}

try {
    const firebaseServiceAccountPath = './config/firebase-service-account-key.json';

    if (existsSync(firebaseServiceAccountPath)) {
        const serviceAccount = JSON.parse(
            await readFile(firebaseServiceAccountPath, 'utf8')
        );

        if (!admin.apps.length) {
            const config = getConfig();
            firebaseApp = admin.initializeApp({
                credential: admin.credential.cert(serviceAccount),
                storageBucket: config.bucketName
            });
        } else {
            firebaseApp = admin.app();
        }
        storage = new Storage({
            credentials: serviceAccount
        });
    } else {
        if (!admin.apps.length) {
            const config = getConfig();
            firebaseApp = admin.initializeApp({
                storageBucket: config.bucketName
            });
        } else {
            firebaseApp = admin.app();
        }
        storage = new Storage();
    }
} catch (error) {
    console.error('Firebase初期化エラー:', error);
    process.exit(1);
}

const bucketName = getConfig().bucketName;
const bucket = storage.bucket(bucketName);

async function warmupMiddleware(_, res) {
    try {
        await getConfig();
        await bucket.exists();
        console.log('ウォームアップが完了しました');
        res.status(200).send('OK');
    } catch (error) {
        console.error('ウォームアップエラー:', error);
        res.status(500).send('ウォームアップに失敗しました');
    }
}

async function authMiddleware(req, res, next) {
    const idToken = req.headers.authorization?.split('Bearer ')[1];
    if (!idToken) {
        return res.status(401).json({ error: '認証が必要です' });
    }

    try {
        const decodedToken = await admin.auth().verifyIdToken(idToken);
        const config = getConfig();

        if (!config.allowedEmails.includes(decodedToken.email)) {
            return res.status(403).json({
                error: 'このメールアドレスには操作権限がありません。config.jsonを確認してください。'
            }); 
        }

        req.user = decodedToken;
        next();
    } catch (error) {
        res.status(401).json({ error: '無効な認証トークンです' });
    }
}

async function checkAuthAndAllowedEmail(req, res, next) {
    const sessionCookie = req.cookies.session || '';

    try {
        if (!sessionCookie) {
            return res.redirect('/login');
        }

        const decodedClaim = await admin.auth().verifySessionCookie(sessionCookie, true);
        const config = getConfig();

        if (!config.allowedEmails.includes(decodedClaim.email)) {
            return res.status(403).send('アクセス権限がありません');
        }

        req.user = decodedClaim;
        next();
    } catch (error) {
        res.redirect('/login');
    }
}

function noCacheMiddleware(_, res, next) {
    res.set({
        'Cache-Control': 'no-store, no-cache, must-revalidate, proxy-revalidate',
        'Pragma': 'no-cache',
        'Expires': '0'
    });
    next();
}

function sessionCheckMiddleware(req, res, next) {
    const publicPaths = [
        '/login',
        '/api/firebase-config',
        '/sessionLogin',
        '/logout',
        '/receipt',
        '/api/receipt'
    ];

    if (req.path.startsWith('/api/receipt/') || publicPaths.includes(req.path)) {
        return next();
    }

    const sessionCookie = req.cookies.session;
    if (!sessionCookie) {
        return res.redirect('/login');
    }
    next();
}

function setupServer() {
    const app = express();

    app.set('trust proxy', 1);

    app.use(express.json());
    app.use(cookieParser());

    app.use((req, _, next) => {
        const userIP = req.headers['cf-connecting-ip'] || req.ip;
        const method = req.method;
        const path = req.path;
        console.log(`${method} ${userIP} ${path}`);
        next();
    });

    app.use((req, res, next) => {
        const config = getConfig();
        const allowedOrigins = [
            'receipt-printer.lll.fish',
            'http://localhost:8080',
            `https://${config.printerIP}`,
            `http://${config.printerIP}`
        ];
        const origin = req.headers.origin;

        if (allowedOrigins.includes(origin)) {
            res.header('Access-Control-Allow-Origin', origin);
        }

        res.header('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
        res.header('Access-Control-Allow-Headers', 'Origin, X-Requested-With, Content-Type, Accept, Authorization, SOAPAction, X-CSRF-Token');
        res.header('Access-Control-Allow-Credentials', 'true');
        res.header('Cross-Origin-Opener-Policy', 'unsafe-none');
        res.header('Cross-Origin-Embedder-Policy', 'unsafe-none');

        if (req.method === 'OPTIONS') {
            res.header('Access-Control-Max-Age', '86400');
            return res.status(204).end();
        }
        next();
    });

    app.use(session({
        secret: process.env.SESSION_SECRET || 'your-secret-key',
        resave: false,
        saveUninitialized: false,
        cookie: {
            secure: process.env.NODE_ENV === 'production',
            httpOnly: true,
            sameSite: 'Lax',
            maxAge: 1000 * 60 * 60 * 24
        },
        name: '__Host-session',
        rolling: true
    }));

    const csrfMiddleware = (req, res, next) => {
        const publicPaths = [
            '/login',
            '/api/firebase-config',
            '/sessionLogin',
            '/logout',
            '/receipt',
            '/api/receipt'
        ];

        // プリンターからのリクエストの場合はCSRF検証をスキップ
        const printerIP = getConfig().printerIP;
        const clientIP = req.headers['cf-connecting-ip'] || 
                        req.headers['x-forwarded-for'] || 
                        req.connection.remoteAddress;
        
        // プリンターからのリクエスト判定を強化
        if (req.hostname === printerIP || 
            req.headers['x-forwarded-host'] === printerIP ||
            req.get('origin')?.includes(printerIP) ||
            clientIP === printerIP ||
            publicPaths.includes(req.path)) {
            return next();
        }

        if (req.method === 'GET' || 
            req.path.startsWith('/api/receipt/')) {
            return next();
        }

        const csrfToken = req.headers['x-csrf-token'];
        const cookieToken = req.cookies['csrf-token'];

        if (!csrfToken || !cookieToken || csrfToken !== cookieToken) {
            console.log('CSRF検証失敗:', {
                path: req.path,
                clientIP,
                origin: req.get('origin'),
                hostname: req.hostname
            });
            return res.status(403).json({ error: 'CSRF検証に失敗しました' });
        }

        next();
    };

    app.get('/api/csrf-token', (_, res) => {
        const token = crypto.randomBytes(32).toString('hex');
        res.cookie('csrf-token', token, {
            httpOnly: true,
            secure: process.env.NODE_ENV === 'production',
            sameSite: 'lax'
        });
        res.json({ csrfToken: token });
    });

    app.use(csrfMiddleware);

    app.use(helmet({
        crossOriginEmbedderPolicy: false,
        crossOriginOpenerPolicy: false,
        crossOriginResourcePolicy: { policy: "cross-origin" },
        contentSecurityPolicy: {
            directives: {
                defaultSrc: ["'self'"],
                scriptSrc: [
                    "'self'",
                    "'unsafe-inline'",
                    "'unsafe-eval'",
                    "https://www.gstatic.com",
                    "https://*.googleapis.com",
                    "https://apis.google.com",
                    "https://accounts.google.com"
                ],
                scriptSrcAttr: ["'unsafe-inline'"],
                scriptSrcElem: [
                    "'self'",
                    "'unsafe-inline'",
                    "'unsafe-eval'",
                    "https://www.gstatic.com",
                    "https://*.googleapis.com",
                    "https://apis.google.com",
                    "https://accounts.google.com",
                    "https://*.cloudflareinsights.com"
                ],
                styleSrc: [
                    "'self'",
                    "'unsafe-inline'",
                    "https://www.gstatic.com",
                    "https://*.googleapis.com"
                ],
                imgSrc: ["'self'", "data:", "https:", "blob:"],
                connectSrc: [
                    "'self'",
                    "https://identitytoolkit.googleapis.com",
                    "https://securetoken.googleapis.com",
                    "https://*.googleapis.com",
                    "https://www.googleapis.com",
                    "https://apis.google.com",
                    "https://accounts.google.com",
                    "https://*.lll.fish",
                    "https://*.cloudflareinsights.com",
                    "https://kitchen-printer.lll.fish",
                    `https://${getConfig().printerIP}`
                ],
                frameSrc: [
                    "'self'",
                    "https://accounts.google.com",
                    "https://*.firebaseapp.com",
                    "https://*.googleapis.com"
                ],
                formAction: [
                    "'self'",
                    "https://accounts.google.com"
                ],
                objectSrc: ["'none'"],
                workerSrc: ["'self'", "blob:"],
                childSrc: ["'self'", "blob:", "https://accounts.google.com"],
                baseUri: ["'self'"]
            }
        },
        originAgentCluster: true,
        strictTransportSecurity: {
            maxAge: 15552000,
            includeSubDomains: true,
            preload: true
        },
        referrerPolicy: { policy: "strict-origin-when-cross-origin" },
        noSniff: true,
        xssFilter: true,
        hidePoweredBy: true
    }));

    app.use(sessionCheckMiddleware);

    app.use('/js', express.static(path.join(__dirname, 'js')));
    app.use('/editor', express.static(path.join(__dirname, 'editor')));
    app.use('/login', express.static(path.join(__dirname, 'views/login.html')));
    app.use('/receipt', express.static(path.join(__dirname, 'views/receipt.html')));

    const authCheckMiddleware = async (req, res, next) => {
        const publicPaths = [
            '/login',
            '/receipt',
            '/api/receipt',
            '/api/firebase-config',
            '/sessionLogin',
            '/logout',
            '/_ah/warmup',
            '/editor'
        ];

        if (publicPaths.includes(req.path) ||
            req.path.startsWith('/api/receipt/') ||
            req.path.startsWith('/receipt') ||
            req.path.startsWith('/editor')) {
            return next();
        }

        const sessionCookie = req.cookies.session || '';

        try {
            if (!sessionCookie) {
                return res.redirect('/login');
            }

            const decodedClaim = await admin.auth().verifySessionCookie(sessionCookie, true);
            const config = getConfig();

            if (!config.allowedEmails.includes(decodedClaim.email)) {
                return res.redirect('/login');
            }

            req.user = decodedClaim;
            next();
        } catch (error) {
            console.error('認証エラー:', error);
            return res.redirect('/login');
        }
    };

    app.use(authCheckMiddleware);

    app.get('/receipt', (_, res) => {
        res.sendFile(path.join(__dirname, 'views/receipt.html'));
    });

    app.get('/login', (_, res) => {
        res.sendFile(path.join(__dirname, 'views/login.html'));
    });

    app.use(express.static(path.join(__dirname, 'views')));

    app.post('/sessionLogin', async (req, res) => {
        const idToken = req.body.idToken;

        try {
            const decodedToken = await admin.auth().verifyIdToken(idToken);
            const config = getConfig();

            if (!config.allowedEmails.includes(decodedToken.email)) {
                await admin.auth().revokeRefreshTokens(decodedToken.uid);
                return res.status(403).json({
                    error: 'アクセス権限がありません',
                    action: 'logout'
                });
            }

            const expiresIn = 60 * 60 * 24 * 5 * 1000;
            const sessionCookie = await admin.auth()
                .createSessionCookie(idToken, { expiresIn });

            res.cookie('session', sessionCookie, {
                maxAge: expiresIn,
                httpOnly: true,
                secure: true,
                sameSite: 'strict'
            });

            res.json({ status: 'success' });
        } catch (error) {
            res.status(401).json({ error: '無効な認証トークンです' });
        }
    });

    app.get('/logout', async (req, res) => {
        const sessionCookie = req.cookies.session || '';
        res.clearCookie('session');

        if (sessionCookie) {
            try {
                const decodedClaim = await admin.auth()
                    .verifySessionCookie(sessionCookie);
                await admin.auth().revokeRefreshTokens(decodedClaim.sub);
            } catch (error) {
                console.error('セッション無効化エラー:', error);
            }
        }

        res.redirect('/login');
    });

    app.get('/', noCacheMiddleware, checkAuthAndAllowedEmail, (req, res) => {
        const csrfToken = crypto.randomBytes(32).toString('hex');
        res.cookie('csrf-token', csrfToken, {
            httpOnly: true,
            secure: process.env.NODE_ENV === 'production',
            sameSite: 'lax'
        });
        const html = readFileSync(path.join(__dirname, 'index.html'), 'utf8')
            .replace('content=""', `content="${csrfToken}"`);
        res.send(html);
    });

    app.get('/config/config.json', noCacheMiddleware, checkAuthAndAllowedEmail, (_, res) => {
        res.json(getConfig());
    });

    app.get('/_ah/warmup', warmupMiddleware);

    app.get('/receipt', (req, res) => {
        res.sendFile(path.join(__dirname, 'views', 'receipt.html'));
    });

    app.get('/api/receipt/:uuid', async (req, res) => {
        try {
            const uuid = req.params.uuid;

            const file = bucket.file(`receipts/${uuid}.json`);
            console.log(`バケットパス: receipts/${uuid}.json`);

            const [exists] = await file.exists();
            if (!exists) {
                console.log(`ファイルが存在しません: receipts/${uuid}.json`);
                return res.status(404).json({
                    success: false,
                    error: '領収書が見つかりせん'
                });
            }

            const [content] = await file.download();
            const receiptData = JSON.parse(content.toString());
            console.log('領収書データを正常に取得しました');

            res.json(receiptData);
        } catch (error) {
            console.error('Receipt fetch error:', error);
            res.status(500).json({
                success: false,
                error: 'サーバーエラーが発生しました',
                details: error.message
            });
        }
    });

    app.post('/api/save-receipt', csrfMiddleware, authMiddleware, async (req, res) => {
        try {
            const { uuid, amount, datetime, phone, address, issuerName } = req.body;

            const receiptData = {
                uuid,
                amount,
                datetime,
                phone,
                address,
                issuerName,
            };

            const file = bucket.file(`receipts/${uuid}.json`);
            await file.save(JSON.stringify(receiptData, null, 2), {
                contentType: 'application/json',
                metadata: {
                    cacheControl: 'public, max-age=31536000',
                },
            });

            res.json({ success: true });
        } catch (error) {
            console.error('Receipt save error:', error);
            res.status(500).json({ success: false, error: error.message });
        }
    });

    app.get('/api/generate-uuid', (_, res) => {
        try {
            const uuid = uuidv7();
            res.json({ uuid });
        } catch (error) {
            console.error('UUID生成エラー:', error);
            res.status(500).json({
                error: 'UUIDの生成に失敗しました'
            });
        }
    });

    app.get('/api/firebase-config', (_, res) => {
        const config = getConfig();
        res.json({
            apiKey: config.firebaseApiKey,
            authDomain: config.firebaseAuthDomain,
            projectId: config.firebaseProjectId
        });
    });

    app.use((err, req, res, next) => {
        if (err.code === 'CSRF_INVALID') {
            return res.status(403).json({
                error: 'CSRF検証に失敗しました'
            });
        }

        console.error('サーバーエラー:', err);
        res.status(500).json({
            error: 'サーバーエラー'
        });
    });

    return app;
}

watch('./config/config.json', (eventType) => {
    if (eventType === 'change') {
        try {
            config = undefined;
            getConfig();
            console.log('設定ファイルを再読み込みしました');
        } catch (error) {
            console.error('設定ファイルの読み込みに失敗しました:', error);
        }
    }
});

const app = setupServer();
app.listen(PORT, () => {
    console.log(`Server running at http://localhost:${PORT}`);
});