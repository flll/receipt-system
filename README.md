# ePOS領収書印刷システム
## 概要

このシステムは、EPSONのePOSプリンターを使用して領収書を印刷し、その内容をGoogle Cloud Storageに保存するウェブアプリケーションです。

主な機能：
- Google認証によるセキュアなログイン
- 許可されたメールアドレスのみアクセス可能
- 領収書の印刷と保存
- 印刷された領収書のオンライン確認機能
- QRコードによる領収書の共有

## システム要件

- Node.js 22.11.0以上
- EPSONのePOS対応プリンター
- Google Cloud Platform アカウント
- Firebase プロジェクト

## インストール

### 大まかな流れ:

1. firebaseを作成
1. firebaseでAuthenticatorを有効化
1. firebaseのサービスアカウントのメールアドレスが発行されるため、
    そのメールアドレスを使ってStorage Cloudバケットに読書権限を付与する。
1. config.jsonを頑張って書き込む
1. Access-Control-Allow-Originや、CORSを任意で書き換える
1. Google Cloud Secret-Managerにconfig.jsonを添付する。
1. Cloud Runにビルドしたdockerコンテナを積載し、/app/configにシークレットをマウントさせる。
1. デプロイ

### 詳細な手順:

1. 設定ファイルの準備:
   - `config/config.json.temp` を `config/config.json` にコピー
   - 必要な設定を行う:
     ```json
     {
       "protocol": "https",
       "printerIP": "xxx.xxx.xxx.xxx",
       "devid": "local_printer",
       "phone": "xxx-xxxx-xxxx",
       "address": "東京都...",
       "issuerName": "株式会社...",
       "bucketName": "your-bucket-name",
       "firebaseApiKey": "your-api-key",
       "firebaseAuthDomain": "your-project-id.firebaseapp.com",
       "firebaseProjectId": "your-project-id",
       "allowedEmails": ["example@domain.com"]
     }
     ```

1. Firebase認証の設定:
   - Firebaseコンソールでサービスアカウントキーを取得
   - `config/firebase-service-account-key.json`として保存
   - cloud run の場合はjsonは不要。サービスアカウントの指定を行うこと。

## 使用方法

1. サーバーの起動:

```bash
npm start
```

2. ブラウザでアクセス:
   - 開発環境: `http://localhost:8080`
   - 本番環境: 設定したドメイン

3. 操作手順:
   a. Googleアカウントでログイン
   b. 金額を入力（0-20,000円）
   c. 「印刷」ボタンをクリック


### ストレージセキュリティ
- Google Cloud Storageのバケットアクセス権限の確認
- 保存データの暗号化状態の確認
- 古いデータの自動削除ポリシーの確認

### 推奨される定期的なセキュリティチェック項目
1. 依存パッケージの脆弱性スキャン
```bash
npm run security-full
```

2. コードの静的解析
```bash
npm install -g eslint
eslint .
```

## 技術仕様

### フロントエンド
- HTML/CSS/JavaScript
- Firebase Authentication SDK

### バックエンド
- Express.js
- Firebase Admin SDK
- Google Cloud Storage
- EPSONのePOS SDK

### データストレージ
- Google Cloud Storage（領収書データ）
- Firebaseセッション管理

### 環境変数
- `PORT`: サーバーポート（デフォルト: 8080）

## トラブルシューティング

1. プリンター接続エラー:
   - プリンターのIPアドレスを確認
   - ネットワーク接続を確認
   - ファイアウォール設定を確認

2. 認証エラー:
   - Firebaseの設定を確認
   - 許可メールアドレスリストを確認

![](https://raw.githubusercontent.com/flll/receipt-system/refs/heads/main/editor/b.png)

## ライセンス・利用規約

本リポジトリには、EPSONのePOS SDKが含まれています。  
ePOS SDKの利用には、同梱の `EULA.ja.txt`（エンドユーザーライセンス契約書）の条件が適用されます。  
本SDKを利用する場合は、必ず `EULA.ja.txt` の内容をご確認ください。
