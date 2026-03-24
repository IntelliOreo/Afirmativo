import { createClient } from "./db.js";

export async function seedCoupon(testDatabaseUrl: string, couponCode: string): Promise<void> {
  const client = await createClient(testDatabaseUrl);
  try {
    await client.query(
      `INSERT INTO coupons (code, max_uses, source)
       VALUES ($1, 100, 'e2e')
       ON CONFLICT (code) DO UPDATE
       SET max_uses = EXCLUDED.max_uses,
           source = EXCLUDED.source`,
      [couponCode],
    );
  } finally {
    await client.end();
  }
}
