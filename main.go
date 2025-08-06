package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatalf("データベースの初期化に失敗しました: %v", err)
	}
	defer db.Close()

	// バックグラウンドで定期的な価格チェックと通知処理を実行
	go func() {
		// 例: 1時間ごとにチェック
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			checkAndNotify(db)
		}
	}()

	// Webサーバーを起動
	http.HandleFunc("/api/alerts", func(w http.ResponseWriter, r *http.Request) {
		// CORSヘッダーをここで一元管理
		w.Header().Set("Access-Control-Allow-Origin", "*") // 本番環境では 'http://localhost:3000' のように限定してください
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// プリフライトリクエスト(OPTIONS)に対応
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch r.Method {
		case "POST":
			handleCreateAlert(w, r, db)
		case "GET":
			handleGetAlerts(w, r, db)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	fmt.Println("サーバーがポート8080で起動しました...")
	// この行が実行されると、プログラムはここでブロックされ、サーバーが実行され続けます
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// checkAndNotify はデータベースのアラートをチェックし、通知を行います
func checkAndNotify(db *sql.DB) {
	alerts, err := getActiveAlerts(db)
	if err != nil {
		log.Printf("アクティブなアラートの取得に失敗しました: %v", err)
		return
	}

	if len(alerts) == 0 {
		fmt.Println("チェックするアラートがありません。")
		return
	}

	fmt.Printf("%d件のアラートをチェックします...\n", len(alerts))

	for _, alert := range alerts {
		hotelInfo, err := scrapeHotelInfo(alert.HotelURL)
		if err != nil {
			log.Printf("価格のスクレイピングに失敗しました (%s): %v", alert.HotelURL, err)
			continue
		}

		fmt.Printf("ホテル「%s」(ID:%d): 現在の価格 %d円, 目標価格 %d円\n", hotelInfo.Name, alert.ID, hotelInfo.Price, alert.TargetPrice)

		if hotelInfo.Price <= alert.TargetPrice {
			email, err := getUserEmail(db, alert.UserID)
			if err != nil {
				log.Printf("ユーザーのメールアドレス取得に失敗しました: %v", err)
				continue
			}

			err = sendNotification(email, alert.HotelURL, hotelInfo.Price, hotelInfo.Name)
			if err != nil {
				log.Printf("通知メールの送信に失敗しました: %v", err)
				continue
			}

			// 通知後はアラートを無効化する
			err = deactivateAlert(db, alert.ID)
			if err != nil {
				log.Printf("アラートの無効化に失敗しました: %v", err)
			}
		}
	}
}

// initDB はデータベースに接続し、スキーマを初期化します
func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./hotel_alerts.db")
	if err != nil {
		return nil, fmt.Errorf("データベース接続エラー: %w", err)
	}

	schema, err := os.ReadFile("./database/schema.sql")
	if err != nil {
		return nil, fmt.Errorf("スキーマファイルの読み込みエラー: %w", err)
	}

	if _, err := db.Exec(string(schema)); err != nil {
		return nil, fmt.Errorf("スキーマ実行エラー: %w", err)
	}

	fmt.Println("データベースの初期化が完了しました。")
	return db, nil
}

// User はユーザー情報を表す構造体です
type User struct {
	ID        int
	Email     string
	CreatedAt string
}

// Alert はアラート設定を表す構造体です
type Alert struct {
	ID          int
	UserID      int
	HotelURL    string
	TargetPrice int
	IsActive    bool
	CreatedAt   string
}

// insertUser は新しいユーザーをデータベースに挿入します
func insertUser(db *sql.DB, email string) (*User, error) {
	stmt, err := db.Prepare("INSERT INTO users(email, created_at) VALUES(?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(email, time.Now().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{ID: int(id), Email: email, CreatedAt: time.Now().Format(time.RFC3339)}, nil
}

// insertAlert は新しいアラートをデータベースに挿入します
func insertAlert(db *sql.DB, userID int, hotelURL string, targetPrice int) (*Alert, error) {
	stmt, err := db.Prepare("INSERT INTO alerts(user_id, hotel_url, target_price, is_active, created_at) VALUES(?, ?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(userID, hotelURL, targetPrice, 1, time.Now().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Alert{
		ID:          int(id),
		UserID:      userID,
		HotelURL:    hotelURL,
		TargetPrice: targetPrice,
		IsActive:    true,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}, nil
}

// handleCreateAlert はアラート登録APIのエンドポイントです
func handleCreateAlert(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	if r.Method != "POST" {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	email := r.FormValue("email")
	hotelURL := r.FormValue("hotelUrl")
	targetPriceStr := r.FormValue("targetPrice")
	targetPrice := 0
	if targetPriceStr != "" {
		_, err := fmt.Sscan(targetPriceStr, &targetPrice)
		if err != nil {
			http.Error(w, "Invalid target price", http.StatusBadRequest)
			return
		}
	}

	// ユーザーの重複をチェック
	user, err := getUserByEmail(db, email)
	if err != nil {
		log.Printf("ユーザー検索エラー: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// ユーザーが存在しない場合、新しく作成
		user, err = insertUser(db, email)
		if err != nil {
			log.Printf("ユーザー挿入エラー: %v", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
	}

	// 取得したユーザーIDを使ってアラートを挿入
	_, err = insertAlert(db, user.ID, hotelURL, targetPrice)
	if err != nil {
		log.Printf("アラート挿入エラー: %v", err)
		http.Error(w, "Failed to create alert", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "アラートが正常に登録されました。")
}

// HotelInfo はスクレイピングで取得したホテルの情報を保持します
type HotelInfo struct {
	Price int
	Name  string
}

// scrapeHotelInfo は指定されたホテルの現在の価格と名前をスクレイピングします
func scrapeHotelInfo(url string) (*HotelInfo, error) {
	// http.Getではヘッダーをカスタマイズできないため、http.Clientを使用します
	client := &http.Client{
		// タイムアウトを設定して、リクエストが長引くのを防ぎます
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("リクエストの作成に失敗しました: %w", err)
	}

	// 一般的なブラウザのUser-Agentを設定します
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTPリクエストに失敗しました: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("ステータスコードが異常です: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, fmt.Errorf("HTML解析に失敗しました: %w", err)
	}

	var priceStr string
	var selector string
	var name string

	// URLに基づいてサイトを判別し、適切なセレクタを選択
	if strings.Contains(url, "travel.rakuten.co.jp") {
		// 楽天トラベルのセレクタ (複数の可能性に対応)
		// ホテル名 (ホテルトップページ用とプランページ用)
		name = doc.Find("#htlName").Text()
		if name == "" {
			name = doc.Find("h1.head-hotel-name").Text()
		}
		name = strings.TrimSpace(name)

		// 価格 (検索結果ページ用とプラン詳細ページ用のセレクタを両方試す)
		selector = ".price--num, .rm-prc-prc"
		priceStr = doc.Find(selector).First().Text()
	} else if strings.Contains(url, "booking.com") {
		// Booking.comの価格セレクタの例
		selector = "[data-testid='price-and-discounted-price']"
		priceStr = doc.Find(selector).First().Text()
		name = strings.TrimSpace(doc.Find(".d2fee87262.pp-header__title").Text())
	} else {
		// フォールバックまたは他のサイト用のセレクタ
		selector = ".roomType-charge-price" // 元のセレクタ
		priceStr = doc.Find(selector).First().Text()
		name = "Unknown Hotel"
	}

	if priceStr == "" {
		return nil, fmt.Errorf("価格情報が見つかりませんでした。セレクタ '%s' を確認してください。", selector)
	}

	// 数字のみを抽出する正規表現
	re := regexp.MustCompile(`[0-9]+`)
	digits := re.FindAllString(priceStr, -1)
	if len(digits) == 0 {
		return nil, fmt.Errorf("価格文字列から数字を抽出できませんでした: '%s'", priceStr)
	}

	// 抽出した数字を結合（例: "￥1,234" -> ["1", "234"] -> "1234"）
	priceStr = strings.Join(digits, "")

	// 文字列を整数に変換
	price, err := strconv.Atoi(priceStr)
	if err != nil {
		return nil, fmt.Errorf("価格のパースに失敗しました: %w (元文字列: '%s')", err, priceStr)
	}

	return &HotelInfo{Price: price, Name: name}, nil
}

func getActiveAlerts(db *sql.DB) ([]Alert, error) {
	rows, err := db.Query("SELECT id, user_id, hotel_url, target_price FROM alerts WHERE is_active = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var alert Alert
		if err := rows.Scan(&alert.ID, &alert.UserID, &alert.HotelURL, &alert.TargetPrice); err != nil {
			return nil, err
		}
		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// getUserEmail は user_id に紐づくユーザーのメールアドレスを取得します
func getUserEmail(db *sql.DB, userID int) (string, error) {
	var email string
	err := db.QueryRow("SELECT email FROM users WHERE id = ?", userID).Scan(&email)
	if err != nil {
		return "", err
	}
	return email, nil
}
func getUserByEmail(db *sql.DB, email string) (*User, error) {
	var user User
	err := db.QueryRow("SELECT id, email, created_at FROM users WHERE email = ?", email).Scan(&user.ID, &user.Email, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // ユーザーが存在しない
		}
		return nil, err
	}
	return &user, nil
}

// sendNotification はユーザーに通知メールを送信します
func sendNotification(email string, hotelURL string, currentPrice int, hotelName string) error {
	// TODO: あなたのメールアドレスとアプリパスワードを設定してください
	from := "あなたのメールアドレス"
	password := "あなたのアプリパスワード"
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"

	to := []string{email}
	subject := fmt.Sprintf("Subject: 【価格アラート】%sの価格が目標を下回りました！\n", hotelName)
	body := fmt.Sprintf(`
「%s」の価格が目標価格を下回りました！

現在の価格: %d円
ホテルURL: %s

今すぐチェックしましょう！
`, hotelName, currentPrice, hotelURL)

	msg := []byte(subject + "\n" + body)

	auth := smtp.PlainAuth("", from, password, smtpHost)
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, to, msg)
	if err != nil {
		return err
	}

	log.Printf("通知メールを送信しました: %s", email)
	return nil
}

// handleGetAlerts はアラート一覧を取得するAPIエンドポイントです
func handleGetAlerts(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	w.Header().Set("Content-Type", "application/json")

	// アクティブなアラートを取得
	alerts, err := getActiveAlerts(db)
	if err != nil {
		http.Error(w, fmt.Sprintf("アラートの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	// フロントエンド用のレスポンス形式に変換
	var responseAlerts []map[string]interface{}
	for _, alert := range alerts {
		// 現在の価格をスクレイピング（エラーが発生した場合は0を設定）
		hotelInfo, err := scrapeHotelInfo(alert.HotelURL)
		currentPrice := 0
		hotelName := "ホテル情報の取得に失敗"
		if err != nil {
			log.Printf("価格のスクレイピングに失敗しました (%s): %v", alert.HotelURL, err)
		} else {
			currentPrice = hotelInfo.Price
			hotelName = hotelInfo.Name
		}

		responseAlert := map[string]interface{}{
			"id":           alert.ID,
			"hotel":        hotelName,
			"currentPrice": currentPrice,
			"targetPrice":  alert.TargetPrice,
			"status":       "active",
		}
		responseAlerts = append(responseAlerts, responseAlert)
	}

	response := map[string]interface{}{
		"success": true,
		"alerts":  responseAlerts,
	}

	json.NewEncoder(w).Encode(response)
}

// deactivateAlert は指定されたアラートを無効化します
func deactivateAlert(db *sql.DB, alertID int) error {
	_, err := db.Exec("UPDATE alerts SET is_active = 0 WHERE id = ?", alertID)
	if err != nil {
		return err
	}
	log.Printf("アラートID %d を無効化しました。", alertID)
	return nil
}
