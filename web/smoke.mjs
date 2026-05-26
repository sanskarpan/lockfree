import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import { fileURLToPath } from "node:url";
import path from "node:path";
import process from "node:process";
import { chromium } from "playwright";

const __filename = fileURLToPath(import.meta.url);
const webDir = path.dirname(__filename);
const repoRoot = path.resolve(webDir, "..");
const port = process.env.PORT || "18081";
const baseURL = `http://127.0.0.1:${port}`;
const smokeUsername = "smoke-user";
const smokePassword = "smoke-pass";
const smokeTenant = "tenant-smoke";
const smokeUsersFile = path.join(os.tmpdir(), `lockfree-smoke-users-${process.pid}.json`);
const smokeDataDir = path.join(os.tmpdir(), `lockfree-smoke-data-${process.pid}`);
const smokeUsers = {
  version: 1,
  users: [
    {
      username: smokeUsername,
      password_hash: "$2a$10$e/OI17fbYzRpDiDcK27BqezDn3vWygFF07V7Xl1/bbGQVu3h/H5Z2",
      tenant: smokeTenant,
      role: "admin",
    },
  ],
};

fs.writeFileSync(smokeUsersFile, JSON.stringify(smokeUsers), "utf8");
fs.rmSync(smokeDataDir, { recursive: true, force: true });

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForServer(url, timeoutMs = 20000) {
  const start = Date.now();
  let lastError;

  while (Date.now() - start < timeoutMs) {
    try {
      const response = await fetch(url, { redirect: "manual" });
      if (response.ok || response.status === 303) {
        return;
      }
      lastError = new Error(`unexpected status ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await sleep(250);
  }

  throw new Error(`server did not become ready at ${url}: ${lastError}`);
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

async function textContent(page, selector) {
  return page.locator(selector).textContent();
}

const server = spawn("go", ["run", "./web"], {
  cwd: repoRoot,
  env: {
    ...process.env,
    PORT: port,
    LOCKFREE_USERS_FILE: smokeUsersFile,
    LOCKFREE_SESSION_SECRET: "smoke-session-secret",
    LOCKFREE_ADMIN_API_TOKEN: "smoke-admin-token",
    LOCKFREE_DATA_DIR: smokeDataDir,
  },
  detached: process.platform !== "win32",
  stdio: ["ignore", "pipe", "pipe"],
});

server.stdout.on("data", (chunk) => process.stdout.write(chunk));
server.stderr.on("data", (chunk) => process.stderr.write(chunk));

let browser;

async function terminateServer(child) {
  if (!child || child.exitCode !== null) {
    return;
  }

  const exitPromise = new Promise((resolve) => child.once("exit", resolve));

  const killChild = (signal) => {
    try {
      if (process.platform === "win32") {
        child.kill(signal);
        return;
      }
      process.kill(-child.pid, signal);
    } catch (error) {
      if (error.code !== "ESRCH") {
        throw error;
      }
    }
  };

  killChild("SIGTERM");
  const exitedGracefully = await Promise.race([
    exitPromise.then(() => true),
    sleep(5000).then(() => false),
  ]);
  if (exitedGracefully) {
    return;
  }

  killChild("SIGKILL");
  await exitPromise;
}

try {
  await waitForServer(`${baseURL}/login`);

  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  const pageErrors = [];
  const consoleErrors = [];

  page.on("pageerror", (error) => {
    pageErrors.push(String(error));
  });
  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });

  await page.goto(`${baseURL}/login`, { waitUntil: "networkidle" });
  await page.locator("#username").fill(smokeUsername);
  await page.locator("#password").fill(smokePassword);
  await page.getByRole("button", { name: "Sign In" }).click();
  await page.waitForFunction(() => document.getElementById("status-text")?.textContent === "Connected");

  assert((await page.title()) === "Lock-Free Data Structures Visualizer", "unexpected page title");
  assert((await textContent(page, "#session-user"))?.trim() === smokeUsername, "session username did not render");
  assert((await textContent(page, "#session-tenant"))?.trim() === smokeTenant, "tenant did not render");

  await page.locator("#stack-input").fill("11");
  await page.getByRole("button", { name: "Push" }).click();
  await page.waitForFunction(() => document.getElementById("stack-length")?.textContent === "1");
  assert((await textContent(page, "#stack-viz .stack-item"))?.trim() === "11", "stack visualization did not update");

  await page.locator("#queue-input").fill("21");
  await page.getByRole("button", { name: "Enqueue" }).click();
  await page.waitForFunction(() => document.getElementById("queue-length")?.textContent === "1");
  assert((await textContent(page, ".queue-items .queue-item"))?.trim() === "21", "queue visualization did not update");

  await page.locator("#rb-input").fill("31");
  await page.getByRole("button", { name: "Write" }).click();
  await page.waitForFunction(() => document.getElementById("rb-length")?.textContent === "1");

  await page.getByRole("button", { name: "Increment" }).click();
  await page.waitForFunction(() => document.getElementById("counter-value")?.textContent === "1");

  await page.locator("#list-input").fill("5");
  await page.getByRole("button", { name: "Insert" }).click();
  await page.waitForFunction(() => document.getElementById("list-length")?.textContent === "1");
  assert((await textContent(page, "#list-viz .list-item"))?.trim() === "5", "list visualization did not update");

  assert(pageErrors.length === 0, `page errors detected: ${pageErrors.join("; ")}`);
  assert(consoleErrors.length === 0, `console errors detected: ${consoleErrors.join("; ")}`);

  console.log("UI smoke test passed");
} finally {
  if (browser) {
    await browser.close();
  }
  try {
    fs.unlinkSync(smokeUsersFile);
  } catch {
    // ignore cleanup errors
  }
  await terminateServer(server);
  try {
    fs.rmSync(smokeDataDir, { recursive: true, force: true });
  } catch {
    // ignore cleanup errors
  }
}
