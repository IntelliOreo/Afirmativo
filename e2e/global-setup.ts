import { createDatabase, prepareDatabase, randomDatabaseName, withDatabaseName } from "./helpers/db.js";
import { seedCoupon } from "./helpers/seed.js";
import { writeRuntimeState } from "./helpers/state.js";

const seededCouponCode = "E2E-TEST-COUPON";

async function globalSetup(): Promise<void> {
  const adminDatabaseUrl = process.env.AFIRMATIVO_TEST_DATABASE_URL?.trim() ?? "";
  if (adminDatabaseUrl === "") {
    throw new Error("AFIRMATIVO_TEST_DATABASE_URL is required");
  }

  const testDatabaseName = randomDatabaseName();
  const testDatabaseUrl = withDatabaseName(adminDatabaseUrl, testDatabaseName);

  await createDatabase(adminDatabaseUrl, testDatabaseName);
  await prepareDatabase(testDatabaseUrl);
  await seedCoupon(testDatabaseUrl, seededCouponCode);

  writeRuntimeState({
    adminDatabaseUrl,
    testDatabaseUrl,
    testDatabaseName,
    seededCouponCode,
  });
}

export default globalSetup;
