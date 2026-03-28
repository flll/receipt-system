# ePOS 領収書印刷システム

EPSON ePOS プリンターで領収書を印刷し、Google Cloud Storage に保存するウェブアプリケーション。
Firebase Authentication による Google ログイン認証付き。印刷された領収書は QR コード経由で公開共有できる。

**Go のシングルバイナリを Docker でビルド・実行するため、ホストに Go のインストールは不要。**

## 必要なもの

| 必須 | 用途 |
|---|---|
| **Docker** | ビルド・実行環境 |
| **make** | コマンドのショートカット |
| **GCP アカウント** | Cloud Storage / Cloud Run |
| **Firebase プロジェクト** | 認証 (Google ログイン) |
| **EPSON ePOS 対応プリンター** | 領収書の印刷 |

## クイックスタート

### 1. 設定ファイルを準備

```bash
cp config/config.json.temp config/config.json
```

`config/config.json` を開き、各項目を埋める（詳細は[設定ファイル](#設定ファイル)を参照）。

### 2. Firebase サービスアカウントキーを配置

Firebase Console → プロジェクト設定 → サービスアカウント → 新しい秘密鍵を生成し、以下に保存:

```
config/firebase-service-account-key.json
```

> Cloud Run で動かす場合はキーファイル不要。サービスアカウントを直接指定する。

### 3. 起動

```bash
make run
```

Docker イメージをビルドし、`http://localhost:8080` でサーバーが起動する。

## 設定ファイル

### config/config.json

| キー | 説明 | 例 |
|---|---|---|
| `protocol` | プリンタ通信プロトコル | `"https"` |
| `printerIP` | ePOS プリンタの IP アドレス | `"192.168.1.100"` |
| `devid` | プリンタデバイス ID | `"local_printer"` |
| `phone` | 領収書に印字する電話番号 | `"03-1234-5678"` |
| `address` | 領収書に印字する住所 | `"東京都..."` |
| `amount` | デフォルト金額 | `"0"` |
| `receiptURL` | 領収書公開 URL のベース | `"https://your-domain.com/receipt?uuid="` |
| `issuerName` | 発行者名 | `"株式会社..."` |
| `projectId` | GCP プロジェクト ID | `"my-project"` |
| `bucketName` | GCS バケット名 | `"my-bucket"` |
| `firebaseApiKey` | Firebase Web API キー | `"AIza..."` |
| `firebaseAuthDomain` | Firebase Auth ドメイン | `"my-project.firebaseapp.com"` |
| `firebaseProjectId` | Firebase プロジェクト ID | `"my-project"` |
| `allowedEmails` | アクセス許可メールアドレス | `["user@example.com"]` |

## Make コマンド

| コマンド | 動作 |
|---|---|
| `make run` | Docker イメージをビルドして起動（`localhost:8080`） |
| `make build` | Docker イメージのビルドのみ |
| `make push` | Docker イメージをビルドして Docker Hub にプッシュ |
   
## 本番デプロイ (Cloud Run)

1. **Firebase プロジェクト作成** → Authentication で Google プロバイダを有効化
2. **GCS バケット作成** → Firebase サービスアカウントに読み書き権限を付与
3. **Firebase Console** → Authentication → Settings → Authorized domains に本番ドメインを追加
4. **`config.json` を作成** → 全キーを本番値で埋める
5. **Secret Manager** に `config.json` を登録
6. **Cloud Run にデプロイ**:
   - イメージ: `fjlli/receipt-system`
   - シークレットを `/app/config/config.json` にマウント
   - サービスアカウントに Firebase Admin + GCS 権限を付与

> `main` ブランチに push すると GitHub Actions が自動で Docker Hub にイメージをプッシュする。

## トラブルシューティング

**プリンター接続エラー**
- プリンターの IP アドレスが `config.json` の `printerIP` と一致しているか確認
- ブラウザとプリンターが同一ネットワークにあるか確認
- ファイアウォールでプリンターのポートがブロックされていないか確認

**認証エラー**
- Firebase Console で対象ドメインが Authorized domains に登録されているか確認
- `config.json` の `allowedEmails` に対象メールアドレスが含まれているか確認
- `firebase-service-account-key.json` が正しいプロジェクトのものか確認

## 技術スタック

| 層 | 技術 |
|---|---|
| バックエンド | Go (net/http 標準ライブラリ) |
| フロントエンド | HTML / CSS / JavaScript |
| 認証 | Firebase Authentication |
| ストレージ | Google Cloud Storage |
| プリンター | EPSON ePOS SDK 2.27.0 |
| コンテナ | Docker (distroless/static, ~50MB) |
| CI/CD | GitHub Actions → Docker Hub |
| ホスティング | Google Cloud Run |

![](https://raw.githubusercontent.com/flll/receipt-system/refs/heads/main/editor/b.png)

## ライセンス

本リポジトリには EPSON ePOS SDK が含まれています。
ePOS SDK の利用には `EULA.ja.txt`（エンドユーザーライセンス契約書）の条件が適用されます。
利用前に必ず内容を確認してください。
