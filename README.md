# 電子領収書印刷システム

## 概要
このシステムは、EPSONのレシートプリンターを使用して領収書を印刷し、Google Cloud Storageにデータを保存する機能を提供します。印刷された領収書にはQRコードとPDF417バーコードが含まれ、オンラインで領収書の詳細を確認することができます。

## 主な機能
- 領収書の印刷
- QRコードとPDF417バーコードの自動生成
- Google Cloud Storageへのデータ保存
- オンラインでの領収書確認機能
- UUIDv7による一意の識別子生成

## 必要条件
- Node.js (v14以上)
- EPSONレシートプリンター（ePOS対応機種）
- Google Cloud Platform アカウント
- node -v
  v22.11.0
- npm -v
  10.9.0

## インストール方法

1. リポジトリをクローン

```bash
git clone [リポジトリURL]
cd receipt-printer
```

2. 依存パッケージのインストール

```bash
npm install
```

3. 設定ファイルの作成
`config.json`を以下の形式で作成してください：

```json
{
  "bucketName": "your-bucket-name",
  "printerIP": "192.168.x.x",
  "protocol": "http",
  "devid": "local_printer",
  "amount": "10000",
  "phone": "03-xxxx-xxxx",
  "address": "東京都...",
  "issuerName": "発行者名",
  "receiptURL": "http://your-domain.com/receipt"
}
```

## 使用方法

### サーバーの起動

両方のサーバーを起動：

```bash
npm start
```

表示用サーバーのみ起動：

```bash
npm run start:view
```

保存用サーバーのみ起動：

```bash
npm run start:save
```

### アクセス方法
- 印刷画面: `http://localhost:3000`
- 領収書確認画面: `http://localhost:3000/receipt?uuid=[UUID]`

## ポート設定
- 表示用サーバー: 3000番ポート
- 保存用サーバー: 3001番ポート

## セキュリティ注意事項
- 本番環境では適切な認証・認可の実装が必要です
- Google Cloud Storageの認証情報は適切に管理してください
- プリンターのネットワークアクセスは適切に制限してください

## ライセンス
MITライセンス

## 注意事項
- 本システムは内部ネットワークでの使用を想定しています
- 金額は0円から20,000円までの範囲で設定可能です

## Dockerでの実行方法

```bash
docker build -t receipt-printer .
```

```bash
docker run -p 3000:3000 -p 3001:3001 -v $(pwd):/app receipt-printer
```
