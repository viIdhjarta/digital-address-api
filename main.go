package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// 日本郵便APIのトークンレスポンス
type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

// 日本郵便のデジタル住所APIを使用して住所を検索するハンドラー
func SearchCodeHandler(w http.ResponseWriter, r *http.Request) {
	
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")


	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 検索コード取得
	var searchCode string
	query := r.URL.Query()
	if sc, ok := query["search_code"]; ok && len(sc) > 0 {
		searchCode = sc[0]
	} else {
		// URLパスから取得
		path := strings.TrimPrefix(r.URL.Path, "/")
		searchCode = path
	}

	// 検索コード検証
	match, _ := regexp.MatchString(`^\w{3,7}$`, searchCode)
	if !match {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// トークン取得
	token, err := getJapanPostToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("トークン取得エラー: %v", err)
		return
	}

	// 日本郵便APIへのリクエスト部分
	apiURL := fmt.Sprintf("https://api.da.pf.japanpost.jp/api/v1/searchcode/%s", searchCode)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("リクエスト作成エラー: %v", err)
		return
	}

	// ヘッダー設定
	req.Header.Set("User-Agent", "Go-http-client/1.1")
	req.Header.Set("Authorization", "Bearer "+token)

	// リクエスト実行
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("APIリクエストエラー: %v", err)
		return
	}
	defer resp.Body.Close()

	// レスポンスのコピー
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// 日本郵便APIのアクセストークンを取得
func getJapanPostToken() (string, error) {
	tokenFilename := "access_token.json"

	
	if _, err := os.Stat(tokenFilename); err == nil {
		data, err := os.ReadFile(tokenFilename)
		if err == nil {
			var tokenResp TokenResponse
			if json.Unmarshal(data, &tokenResp) == nil {
				fileInfo, err := os.Stat(tokenFilename)
				if err == nil {
					modTime := fileInfo.ModTime()
					if time.Since(modTime).Seconds() < float64(tokenResp.ExpiresIn) {
						return tokenResp.Token, nil
					}
				}
			}
		}
	}

	credentials, err := os.ReadFile("credentials.json")
	if err != nil {
		return "", fmt.Errorf("認証情報ファイル読み込みエラー: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.da.pf.japanpost.jp/api/v1/j/token", bytes.NewBuffer(credentials))
	if err != nil {
		return "", fmt.Errorf("トークンリクエスト作成エラー: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Go-http-client/1.1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("トークンAPIリクエストエラー: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("トークン取得エラー: ステータスコード %d, レスポンス: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("トークンレスポンス読み込みエラー: %w", err)
	}

	// トークンの保存
	err = os.WriteFile(tokenFilename, body, 0644)
	if err != nil {
		return "", fmt.Errorf("トークン保存エラー: %w", err)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("トークンJSONパースエラー: %w", err)
	}

	return tokenResp.Token, nil
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/{search_code:[\\w]{3,7}}", SearchCodeHandler).Methods("GET")
	r.HandleFunc("/", SearchCodeHandler).Methods("GET")
	
	log.Println("サーバーを起動中。ポート: 8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
} 