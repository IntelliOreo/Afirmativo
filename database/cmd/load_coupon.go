package cmd

import (
	"flag"
	"fmt"
	"log"
)

// LoadCoupon generates coupon codes and inserts them into the database.
//
// Usage:
//
//	go run main.go load_coupon --prefix BETA --count 50 --max-uses 1 --source beta_testers
func LoadCoupon(dbURL string, args []string) {
	fs := flag.NewFlagSet("load_coupon", flag.ExitOnError)
	prefix := fs.String("prefix", "COUPON", "prefix for generated coupon codes (e.g., BETA → BETA-0001)")
	count := fs.Int("count", 10, "number of coupons to generate")
	maxUses := fs.Int("max-uses", 1, "max uses per coupon")
	discountPct := fs.Int("discount-pct", 100, "discount percentage (100 = free)")
	source := fs.String("source", "", "source label (e.g., beta_testers, development)")

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
	for i := range *count {
		code := fmt.Sprintf("%s-%04d", *prefix, i+1)

		result, err := db.Exec(
			`INSERT INTO coupons (code, max_uses, discount_pct, source)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (code) DO NOTHING`,
			code, *maxUses, *discountPct, *source,
		)
		if err != nil {
			log.Printf("failed to insert coupon %s: %v", code, err)
			continue
		}
		rows, err := result.RowsAffected()
		if err != nil {
			log.Printf("failed to get rows affected for coupon %s: %v", code, err)
			continue
		}
		if rows == 0 {
			skipped++
			continue
		}
		fmt.Println(code)
		inserted++
	}

	fmt.Printf("\n%d inserted, %d skipped (already exist)\n", inserted, skipped)
}
