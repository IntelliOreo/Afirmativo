import fs from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Client } from "pg";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..", "..");
const migrationsDir = path.join(repoRoot, "e2e", "migrations");

export async function createClient(connectionString: string): Promise<Client> {
  const client = new Client({ connectionString });
  await client.connect();
  return client;
}

export function randomDatabaseName(): string {
  return `afirmativo_e2e_${Math.random().toString(16).slice(2, 10)}`;
}

export function withDatabaseName(connectionString: string, databaseName: string): string {
  const url = new URL(connectionString);
  url.pathname = `/${databaseName}`;
  return url.toString();
}

export async function createDatabase(adminDatabaseUrl: string, databaseName: string): Promise<void> {
  const client = await createClient(adminDatabaseUrl);
  try {
    await client.query(`CREATE DATABASE "${databaseName}"`);
  } finally {
    await client.end();
  }
}

export async function dropDatabase(adminDatabaseUrl: string, databaseName: string): Promise<void> {
  const client = await createClient(adminDatabaseUrl);
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

export async function prepareDatabase(testDatabaseUrl: string): Promise<void> {
  const client = await createClient(testDatabaseUrl);
  try {
    await client.query(`CREATE EXTENSION IF NOT EXISTS pgcrypto`);
    const migrationPaths = await getMigrationPaths();
    for (const migrationPath of migrationPaths) {
      const sql = await fs.readFile(migrationPath, "utf8");
      const statements = splitMigrationStatements(sql);
      for (const statement of statements) {
        await client.query(statement);
      }
    }
  } finally {
    await client.end();
  }
}

async function getMigrationPaths(): Promise<string[]> {
  const entries = await fs.readdir(migrationsDir, { withFileTypes: true });
  return entries
    .filter((entry) => !entry.isDirectory() && entry.name.endsWith(".up.sql"))
    .map((entry) => path.join(migrationsDir, entry.name))
    .sort();
}

function splitMigrationStatements(contents: string): string[] {
  const withoutCommentLines = contents
    .split("\n")
    .filter((line) => !line.trimStart().startsWith("--"))
    .join("\n");

  return withoutCommentLines
    .split(";")
    .map((statement) => statement.trim())
    .filter((statement) => statement.length > 0);
}
