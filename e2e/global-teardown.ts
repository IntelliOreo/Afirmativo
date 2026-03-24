import { dropDatabase } from "./helpers/db.js";
import { readRuntimeState, removeRuntimeState } from "./helpers/state.js";

async function globalTeardown(): Promise<void> {
  const state = readRuntimeState();
  await dropDatabase(state.adminDatabaseUrl, state.testDatabaseName);
  removeRuntimeState();
}

export default globalTeardown;
