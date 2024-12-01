import express from 'express';
import path from 'path';
import { Storage } from '@google-cloud/storage';
import { fileURLToPath } from 'url';
import { dirname } from 'path';
import { v7 as uuidv7 } from 'uuid';
import { readFileSync, watch } from 'fs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const DEFAULT_PORT = 3000;
const args = process.argv.slice(2);
const command = args[0];
const portArg = args[1];

const PORT = process.env.PORT ? parseInt(process.env.PORT, 10) : 
             portArg ? parseInt(portArg, 10) : 
             DEFAULT_PORT;

// 設定ファイルの変更を監視
watch('./config.json', (eventType) => {
    if (eventType === 'change') {
        try {
            getConfig(); // 設定を再読み込み
            console.log('設定ファイルを再読み込みしました');
        } catch (error) {
            console.error('設定ファイルの読み込みに失敗しました:', error);
        }
    }
});

// Google Cloud Storageクライアントの初期化
function getConfig() {
    return JSON.parse(readFileSync('./config.json', 'utf8'));
}

const storage = new Storage(); // Cloud Runの環境では自動的に認証されます

const bucketName = getConfig().bucketName;
const bucket = storage.bucket(bucketName);

// 表示用サーバーの設定
function setupViewServer() {
    const viewApp = express();
    viewApp.use(express.static(path.join(__dirname, 'views')));

    viewApp.get('/receipt', (req, res) => {
        res.sendFile(path.join(__dirname, 'views', 'receipt.html'));
    });

    viewApp.get('/api/receipt/:uuid', async (req, res) => {
        try {
            const uuid = req.params.uuid;
            const file = bucket.file(`receipts/${uuid}.json`);

            const [exists] = await file.exists();
            if (!exists) {
                throw new Error('領収書が見つかりません');
            }

            const [content] = await file.download();
            const receiptData = JSON.parse(content.toString());

            res.json(receiptData);
        } catch (error) {
            console.error('Receipt fetch error:', error);
            res.status(404).json({
                success: false,
                error: '領収書が見つかりません'
            });
        }
    });

    viewApp.get('/api/generate-uuid', (req, res) => {
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

    return viewApp;
}

// 保存用サーバーの設定
function setupSaveServer() {
    const saveApp = express();
    saveApp.use(express.json());
    
    saveApp.use(express.static(path.join(__dirname)));

    saveApp.get('/', (_, res) => {
        res.sendFile(path.join(__dirname, 'index.html'));
    });

    saveApp.post('/api/save-receipt', async (req, res) => {
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

    return saveApp;
}

if (process.env.CLOUD_RUN) {
    const viewApp = setupViewServer();
    viewApp.listen(PORT, () => {
        console.log(`Cloud Run: View server starting on port ${PORT}`);
        console.log('Environment:', {
            PORT: process.env.PORT,
            CLOUD_RUN: process.env.CLOUD_RUN,
            NODE_ENV: process.env.NODE_ENV
        });
    }).on('error', (err) => {
        console.error('Server error:', err);
        process.exit(1);
    });
} else {
    if (!command) {
        console.error('使用方法: npm run server [view|save] [port]');
        process.exit(1);
    }

    if (command === 'view') {
        const viewApp = setupViewServer();
        viewApp.listen(PORT, () => {
            console.log(`View server running at http://localhost:${PORT}`);
        });
    }

    if (command === 'save') {
        const saveApp = setupSaveServer();
        saveApp.listen(PORT, () => {
            console.log(`Save server running at http://localhost:${PORT}`);
        });
    }
}
