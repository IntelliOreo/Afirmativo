import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Client } from "pg";

const statePath = process.env.AFIRMATIVO_E2E_STATE_PATH;
if (!statePath) {
  throw new Error("AFIRMATIVO_E2E_STATE_PATH must point to the generated runtime state file");
}

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const seededCouponCode = "E2E-TEST-COUPON";
const adminDatabaseUrl = process.env.AFIRMATIVO_TEST_DATABASE_URL?.trim() ?? "";
if (adminDatabaseUrl === "") {
  throw new Error("AFIRMATIVO_TEST_DATABASE_URL is required");
}

const state = await prepareRuntimeState(adminDatabaseUrl, statePath);
const baseEnv = {
  ...process.env,
  PORT: "8080",
  DATABASE_URL: state.testDatabaseUrl,
  FRONTEND_URL: "http://127.0.0.1:3000",
  JWT_SECRET: "e2e-test-jwt-secret-32-characters!!",
  STRIPE_SECRET_KEY: "sk_test_e2e_fake",
  STRIPE_WEBHOOK_SECRET: "whsec_e2e_test_secret",
  STRIPE_BASE_URL: "http://127.0.0.1:12111",
  AI_PROVIDER: "ollama",
  AI_AREA_CONFIG: JSON.stringify([
    { id: 1, slug: "area_1", label: "Area 1", description: "Area 1", sufficiency_requirements: "Area 1", fallback_question: "Fallback 1?" },
    { id: 2, slug: "area_2", label: "Area 2", description: "Area 2", sufficiency_requirements: "Area 2", fallback_question: "Fallback 2?" },
    { id: 3, slug: "area_3", label: "Area 3", description: "Area 3", sufficiency_requirements: "Area 3", fallback_question: "Fallback 3?" },
    { id: 4, slug: "area_4", label: "Area 4", description: "Area 4", sufficiency_requirements: "Area 4", fallback_question: "Fallback 4?" }
  ]),
  LOG_LEVEL: "warn",
  LOG_FORMAT: "text",
  ADMIN_CLEANUP_ENABLED: "false",
  OTEL_ENABLED: "false",
};

const child = spawn("go", ["run", "./cmd/server"], {
  cwd: path.join(repoRoot, "backend"),
  env: baseEnv,
  stdio: "inherit",
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    child.kill(signal);
  });
}

child.on("exit", (code, signal) => {
  void cleanupRuntimeState(statePath).finally(() => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exit(code ?? 0);
  });
});

async function prepareRuntimeState(adminUrl, filePath) {
  const databaseName = `afirmativo_e2e_${Math.random().toString(16).slice(2, 10)}`;
  const testDatabaseUrl = withDatabaseName(adminUrl, databaseName);

  await createDatabase(adminUrl, databaseName);
  try {
    await prepareDatabase(testDatabaseUrl);
    await seedCoupon(testDatabaseUrl, seededCouponCode);
    const stateValue = {
      adminDatabaseUrl: adminUrl,
      testDatabaseUrl,
      testDatabaseName: databaseName,
      seededCouponCode,
    };
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
    fs.writeFileSync(filePath, JSON.stringify(stateValue, null, 2));
    return stateValue;
  } catch (error) {
    await dropDatabase(adminUrl, databaseName);
    throw error;
  }
}

async function cleanupRuntimeState(filePath) {
  if (!fs.existsSync(filePath)) {
    return;
  }

  const stateValue = JSON.parse(fs.readFileSync(filePath, "utf8"));
  fs.rmSync(filePath, { force: true });
  await dropDatabase(stateValue.adminDatabaseUrl, stateValue.testDatabaseName);
}

async function createClient(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  return client;
}

function withDatabaseName(connectionString, databaseName) {
  const url = new URL(connectionString);
  url.pathname = `/${databaseName}`;
  return url.toString();
}

async function createDatabase(adminUrl, databaseName) {
  const client = await createClient(adminUrl);
  try {
    await client.query(`CREATE DATABASE "${databaseName}"`);
  } finally {
    await client.end();
  }
}

async function dropDatabase(adminUrl, databaseName) {
  const client = await createClient(adminUrl);
  try {
    await client.query(
      `SELECT pg_terminate_backend(pid)
         FROM pg_stat_activity
        WHERE datname = $1
          AND pid <> pg_backend_pid()`,
      [databaseName],
    );
    await client.query(`DROP DATABASE IF EXISTS "${databaseName}"`);
  } finally {
    await client.end();
  }
}

async function prepareDatabase(testDatabaseUrl) {
  const client = await createClient(testDatabaseUrl);
  try {
    await client.query(`CREATE EXTENSION IF NOT EXISTS pgcrypto`);
    const migrationsDir = path.join(repoRoot, "e2e", "migrations");
    const migrationPaths = fs.readdirSync(migrationsDir)
      .filter((name) => name.endsWith(".up.sql"))
      .sort()
      .map((name) => path.join(migrationsDir, name));

    for (const migrationPath of migrationPaths) {
      const sql = fs.readFileSync(migrationPath, "utf8");
      const statements = sql
        .split("\n")
        .filter((line) => !line.trimStart().startsWith("--"))
        .join("\n")
        .split(";")
        .map((statement) => statement.trim())
        .filter((statement) => statement.length > 0);

      for (const statement of statements) {
        await client.query(statement);
      }
    }
  } finally {
    await client.end();
  }
}

async function seedCoupon(testDatabaseUrl, couponCode) {
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
