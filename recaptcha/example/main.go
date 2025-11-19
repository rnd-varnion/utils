package example

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rnd-varnion/utils/recaptcha"
)

type RecaptchaForm struct {
	RecaptchaResponse string `form:"g-recaptcha-response" json:"g-recaptcha-response"`
}

type AuthRequest struct {
	Key           string           `json:"key" binding:"required"`
	Password      string           `json:"password" binding:"required"`
	Type          string           `json:"type"`
	RecaptchaForm `json:",inline"` // Add recaptcha form fields
}

func main() {
	// Initialize reCAPTCHA (enable it (can be config driven) and set your secret key)
	recaptcha.New(true, "YOUR_RECAPTCHA_SECRET_KEY")

	http.HandleFunc("/login", handleAuth)

	fmt.Println("Server running on http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate reCAPTCHA
	if err := recaptcha.Validate(req.RecaptchaResponse); err != nil {
		http.Error(w, fmt.Sprintf("recaptcha validation failed: %v", err), http.StatusForbidden)
		return
	}

	// Continue with authentication logic
	if req.Key == "admin" && req.Password == "secret" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "Login successful"}`))
		return
	}

	http.Error(w, "invalid credentials", http.StatusUnauthorized)
}
