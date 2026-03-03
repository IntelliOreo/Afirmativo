package cmd

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/big"
	"strings"
)

const (
	couponAlphabet      = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	maxGenerateAttempts = 5
)

// LoadCoupon generates coupon codes and inserts them into the database.
//
// Usage:
//
//	go run main.go load_coupon --prefix BETA --count 50 --max-uses 1 --source beta_testers
func LoadCoupon(dbURL string, args []string) {
	fs := flag.NewFlagSet("load_coupon", flag.ExitOnError)
	prefix := fs.String("prefix", "COUPON", "prefix for generated coupon codes (e.g., BETA → BETA-7K9Q2M4X)")
	count := fs.Int("count", 10, "number of coupons to generate")
	maxUses := fs.Int("max-uses", 1, "max uses per coupon")
	discountPct := fs.Int("discount-pct", 100, "discount percentage (100 = free)")
	source := fs.String("source", "", "source label (e.g., beta_testers, development)")
	tokenLength := fs.Int("token-length", 8, "length of the random coupon suffix")

	if err := fs.Parse(args); err != nil {
		log.Fatalf("failed to parse args: %v", err)
	}

	db, err := OpenDB(dbURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Close()

	inserted := 0
	skipped := 0
	failed := 0
	normalizedPrefix := strings.ToUpper(strings.TrimSpace(*prefix))

	for i := 0; i < *count; i++ {
		done := false
		for attempt := 1; attempt <= maxGenerateAttempts; attempt++ {
			code, err := generateCouponCode(normalizedPrefix, *tokenLength)
			if err != nil {
				log.Printf("failed to generate coupon code: %v", err)
				failed++
				done = true
				break
			}

			result, err := db.Exec(
				`INSERT INTO coupons (code, max_uses, discount_pct, source)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (code) DO NOTHING`,
				code, *maxUses, *discountPct, *source,
			)
			if err != nil {
				log.Printf("failed to insert coupon %s: %v", code, err)
				failed++
				done = true
				break
			}
			rows, err := result.RowsAffected()
			if err != nil {
				log.Printf("failed to get rows affected for coupon %s: %v", code, err)
				failed++
				done = true
				break
			}
			if rows == 0 {
				continue
			}
			fmt.Println(code)
			inserted++
			done = true
			break
		}
		if !done {
			skipped++
		}
	}

	fmt.Printf("\n%d inserted, %d skipped (code collisions), %d failed\n", inserted, skipped, failed)
}

func generateCouponCode(prefix string, tokenLength int) (string, error) {
	if tokenLength < 4 {
		return "", fmt.Errorf("token length must be at least 4")
	}

	token, err := randomToken(tokenLength)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s", prefix, token), nil
}

func randomToken(length int) (string, error) {
	var b strings.Builder
	b.Grow(length)

	max := big.NewInt(int64(len(couponAlphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(couponAlphabet[n.Int64()])
	}

	return b.String(), nil
}
