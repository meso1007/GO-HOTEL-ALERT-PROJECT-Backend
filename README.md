# Hotel Price Alert Backend

ホテル価格監視システムのバックエンドAPIサーバーです。指定されたホテルの価格を定期的にチェックし、目標価格に達した際にメール通知を送信します。

## 🚀 機能

- **価格監視**: 指定されたホテルURLの価格を定期的にスクレイピング
- **アラート管理**: ユーザーごとの価格アラート設定と管理
- **メール通知**: 目標価格達成時の自動メール通知
- **RESTful API**: フロントエンドとの連携用API
- **SQLite データベース**: 軽量なデータベースでユーザーとアラート情報を管理
- **対象WebSite**: Rakuten Trvel https://travel.rakuten.co.jp/

## 🛠️ 技術スタック

- **言語**: Go 1.24.5
- **データベース**: SQLite3
- **Webスクレイピング**: goquery
- **HTTP サーバー**: net/http (標準ライブラリ)
- **メール送信**: net/smtp (標準ライブラリ)

## 📋 前提条件

- Go 1.24.5以上
- SQLite3
- メール送信設定（SMTP）

## 🔧 セットアップ

### 1. リポジトリのクローン
```bash
git clone https://github.com/meso1007/GO-HOTEL-ALERT-PROJECT-Backend
cd hotel-price-alert/backend
```

### 2. 依存関係のインストール
```bash
go mod download
```

### 3. 環境変数の設定
```bash
# メール送信設定（必要に応じて）
export SMTP_HOST=smtp.gmail.com
export SMTP_PORT=587
export SMTP_USERNAME=your-email@gmail.com
export SMTP_PASSWORD=your-app-password
```

### 4. データベースの初期化
```bash
# データベースは自動的に作成されます
go run main.go
```

### 5. サーバーの起動
```bash
go run main.go
```

サーバーは `http://localhost:8080` で起動します。

## 📊 データベーススキーマ

### users テーブル
| カラム | 型 | 説明 |
|--------|----|----|
| id | INTEGER | 主キー（自動増分） |
| email | TEXT | ユーザーのメールアドレス（一意） |
| created_at | TEXT | 作成日時 |

### alerts テーブル
| カラム | 型 | 説明 |
|--------|----|----|
| id | INTEGER | 主キー（自動増分） |
| user_id | INTEGER | ユーザーID（外部キー） |
| hotel_url | TEXT | 監視対象ホテルのURL |
| target_price | INTEGER | 目標価格（円） |
| is_active | INTEGER | アクティブ状態（1: 有効, 0: 無効） |
| created_at | TEXT | 作成日時 |

## 🔌 API 仕様

### アラート作成
```
POST /api/alerts
```

**リクエストボディ:**
```json
{
  "email": "user@example.com",
  "hotelUrl": "https://travel.rakuten.co.jp/HOTEL/...",
  "targetPrice": 25000
}
```

**レスポンス:**
```json
{
  "success": true,
  "message": "アラートが正常に登録されました",
  "alert": {
    "id": 1,
    "hotelUrl": "https://travel.rakuten.co.jp/HOTEL/...",
    "targetPrice": 25000,
    "isActive": true,
    "createdAt": "2024-01-01T00:00:00Z"
  }
}
```

## 🏗️ アーキテクチャ

### 主要コンポーネント

1. **メインサーバー** (`main.go`)
   - HTTPサーバーの起動
   - バックグラウンド処理の開始

2. **データベース管理**
   - SQLite3接続とスキーマ初期化
   - ユーザーとアラートのCRUD操作

3. **価格監視システム**
   - 定期的な価格チェック（1分間隔）
   - Webスクレイピングによる価格取得
   - 目標価格達成時の通知処理

4. **メール通知システム**
   - SMTPを使用したメール送信
   - 価格アラート通知の自動送信

### 処理フロー

1. ユーザーがアラートを登録
2. バックグラウンドで定期的に価格をチェック
3. 目標価格に達した場合、メール通知を送信
4. アラートを無効化

## 🔄 バックグラウンド処理

サーバー起動時に以下のバックグラウンド処理が開始されます：

- **価格チェック**: 1分間隔でアクティブなアラートの価格をチェック
- **通知送信**: 目標価格達成時のメール通知
- **アラート管理**: 通知後のアラート無効化

## 🚨 注意事項

- **Webスクレイピング**: 対象サイトの利用規約を確認してください
- **メール送信**: SMTP設定が必要です
- **データベース**: SQLiteファイルは自動的に作成されます
- **エラーハンドリング**: ログファイルでエラーを確認できます

## 🐛 トラブルシューティング

### よくある問題

1. **データベース接続エラー**
   ```bash
   # データベースファイルの権限を確認
   ls -la hotel_alerts.db
   ```

2. **メール送信エラー**
   ```bash
   # SMTP設定を確認
   echo $SMTP_HOST
   echo $SMTP_USERNAME
   ```

3. **ポートが使用中**
   ```bash
   # ポート8080の使用状況を確認
   lsof -i :8080
   ```

## 📝 開発

### コードの構造
```
backend/
├── main.go              # メインアプリケーション
├── go.mod               # Goモジュール定義
├── go.sum               # 依存関係チェックサム
├── database/
│   └── schema.sql       # データベーススキーマ
└── hotel_alerts.db      # SQLiteデータベース（自動生成）
```

### テストの実行
```bash
go test ./...
```

### ビルド
```bash
go build -o hotel-alert-server main.go
```

## 📄 ライセンス

このプロジェクトはMITライセンスの下で公開されています。

## 🤝 コントリビューション

1. このリポジトリをフォーク
2. 機能ブランチを作成 (`git checkout -b feature/amazing-feature`)
3. 変更をコミット (`git commit -m 'Add amazing feature'`)
4. ブランチにプッシュ (`git push origin feature/amazing-feature`)
5. プルリクエストを作成
