import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export type RuntimeState = {
  adminDatabaseUrl: string;
  testDatabaseUrl: string;
  testDatabaseName: string;
  seededCouponCode: string;
};

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const packageRoot = path.resolve(__dirname, "..");
const tempDir = path.join(packageRoot, ".tmp");

export function runtimeStatePath(): string {
  return process.env.AFIRMATIVO_E2E_STATE_PATH
    ? path.resolve(process.env.AFIRMATIVO_E2E_STATE_PATH)
    : path.join(tempDir, "runtime-state.json");
}

export function ensureTempDir(): void {
  fs.mkdirSync(tempDir, { recursive: true });
}

export function writeRuntimeState(state: RuntimeState): void {
  ensureTempDir();
  fs.writeFileSync(runtimeStatePath(), JSON.stringify(state, null, 2));
}

export function readRuntimeState(): RuntimeState {
  const raw = fs.readFileSync(runtimeStatePath(), "utf8");
  return JSON.parse(raw) as RuntimeState;
}

export function removeRuntimeState(): void {
  if (fs.existsSync(runtimeStatePath())) {
    fs.rmSync(runtimeStatePath());
  }
}
