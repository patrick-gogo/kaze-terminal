import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { once } from 'node:events';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer } from 'node:http';
import { CdpPeer, FocusGuardRelay, McpPeer, delay } from './protocol.mjs';

const directory = path.dirname(fileURLToPath(import.meta.url));
const artifacts = path.join(directory, 'artifacts');
await fs.mkdir(artifacts, { recursive: true });
const browsers = new Set();
const relays = new Set();
const mcps = new Set();
const focusEvents = [];
const phases = process.argv.includes('--isolation') ? ['isolation'] : process.argv.includes('--mcp') ? ['mcp'] : ['isolation', 'mcp'];
const report = { date: new Date().toISOString(), platform: process.platform, release: os.release(), arch: os.arch(), node: process.version, nodeExecutable: process.execPath, phases, baseCommit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: directory, encoding: 'utf8' }).trim(), dependencies: JSON.parse(await fs.readFile(path.join(directory, 'package.json'), 'utf8')).dependencies, observations: {}, browserInstances: [] };
let monitor;
let fixture;

function executable() {
  if (process.env.KAZE_SPIKE_CHROME) return process.env.KAZE_SPIKE_CHROME;
  if (process.platform === 'darwin') return '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
  if (process.platform === 'win32') return path.join(process.env.PROGRAMFILES || 'C:\\Program Files', 'Google/Chrome/Application/chrome.exe');
  throw new Error('Set KAZE_SPIKE_CHROME to an installed Chrome executable');
}

async function launchBrowser(label) {
  const started = performance.now();
  const profile = await fs.mkdtemp(path.join(os.tmpdir(), 'kaze-browser-spike-'));
  const flags = [
    `--user-data-dir=${profile}`, '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=0',
    '--no-first-run', '--no-default-browser-check', '--no-startup-window', '--disable-extensions',
    '--disable-background-networking', '--disable-component-update', '--disable-sync', '--disable-default-apps',
    '--metrics-recording-only', '--disable-features=Translate,MediaRouter', '--window-size=900,650'
  ];
  const item = { label, profile, closed: false };
  browsers.add(item);
  if (process.platform === 'darwin') {
    execFileSync('/usr/bin/open', ['-g', '-n', '-a', executable().split('/Contents/')[0], '--args', ...flags], { timeout: 10000 });
  } else {
    item.process = spawn(executable(), flags, { stdio: 'ignore', windowsHide: true });
  }
  let lines;
  const deadline = performance.now() + 10000;
  while (performance.now() < deadline) {
    try { lines = (await fs.readFile(path.join(profile, 'DevToolsActivePort'), 'utf8')).trim().split('\n'); break; } catch { await delay(50); }
  }
  assert.ok(lines, `Chrome did not expose its temporary endpoint: ${label}`);
  item.http = `http://127.0.0.1:${lines[0]}`;
  item.ws = `ws://127.0.0.1:${lines[0]}${lines[1]}`;
  item.cdp = await CdpPeer.connect(item.ws);
  item.version = await item.cdp.call('Browser.getVersion');
  const processInfo = await item.cdp.call('SystemInfo.getProcessInfo');
  item.pid = processInfo.processInfo.find(p => p.type === 'browser').id;
  item.startupMs = Math.round(performance.now() - started);
  item.startedAt = Date.now();
  report.browserInstances.push({ label, pid: item.pid, startupMs: item.startupMs, startedAt: item.startedAt });
  report.chrome = item.version.product;
  return item;
}

async function stopBrowser(item) {
  if (item.closed) return;
  item.closed = true;
  try { await item.cdp.call('Browser.close'); } catch {}
  item.cdp.close();
  for (let n = 0; n < 30; n++) {
    try { process.kill(item.pid, 0); } catch { break; }
    await delay(50);
  }
  // PID was read from this run's private CDP connection, never from a browser name search.
  try { process.kill(item.pid, 0); process.kill(item.pid, 'SIGTERM'); } catch {}
  await delay(150);
  await fs.rm(item.profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
  browsers.delete(item);
}

async function newPage(browser, url, browserContextId) {
  const { targetId } = await browser.cdp.call('Target.createTarget', { url: 'about:blank', newWindow: true, background: true, ...(browserContextId ? { browserContextId } : {}) });
  const { sessionId } = await browser.cdp.call('Target.attachToTarget', { targetId, flatten: true });
  await browser.cdp.call('Page.enable', {}, sessionId);
  await browser.cdp.call('Page.navigate', { url }, sessionId);
  const deadline = performance.now() + 5000;
  while (performance.now() < deadline) {
    const ready = await browser.cdp.call('Runtime.evaluate', { expression: 'document.readyState', returnByValue: true }, sessionId);
    if (ready.result.value === 'complete') break;
    await delay(30);
  }
  return { targetId, sessionId, browserContextId, url };
}

async function evaluate(peer, page, expression) {
  const result = await peer.call('Runtime.evaluate', { expression, returnByValue: true }, page.sessionId);
  return result.result.value;
}

async function isolation(origin) {
  const shared = await launchBrowser('shared-browser');
  const a = await newPage(shared, `${origin}/session-a`);
  const b = await newPage(shared, `${origin}/session-b`);
  await evaluate(shared.cdp, a, "document.cookie='kaze_probe=shared; SameSite=Lax; path=/'");
  const cookieShared = (await evaluate(shared.cdp, b, 'document.cookie')).includes('kaze_probe=shared');
  assert.equal(cookieShared, true);
  const agent = await CdpPeer.connect(shared.ws);
  const targets = (await agent.call('Target.getTargets')).targetInfos;
  assert.ok(targets.some(t => t.targetId === b.targetId));
  const foreign = await agent.call('Target.attachToTarget', { targetId: b.targetId, flatten: true });
  const foreignTitle = await evaluate(agent, foreign, 'document.title');
  assert.equal(foreignTitle, 'Kaze fixture /session-b');
  report.observations.sharedWindows = { sharedSignIn: cookieShared, foreignWindowListed: true, foreignWindowReadable: true, isolated: false, startupMs: shared.startupMs };

  const { browserContextId } = await shared.cdp.call('Target.createBrowserContext');
  const isolated = await newPage(shared, `${origin}/isolated-context`, browserContextId);
  const isolatedCookie = await evaluate(shared.cdp, isolated, 'document.cookie');
  assert.equal(isolatedCookie.includes('kaze_probe'), false);
  const crossContext = await agent.call('Target.attachToTarget', { targetId: isolated.targetId, flatten: true });
  assert.equal(await evaluate(agent, crossContext, 'document.title'), 'Kaze fixture /isolated-context');
  report.observations.separateContexts = { cookiesSeparated: true, foreignContextReadableThroughBrowserEndpoint: true, isolatedConnection: false };

  const pageEndpoint = `ws://127.0.0.1:${new URL(shared.http).port}/devtools/page/${a.targetId}`;
  const pageAgent = await CdpPeer.connect(pageEndpoint);
  try {
    const listed = await pageAgent.call('Target.getTargets');
    const foreignFound = listed.targetInfos.some(t => t.targetId === b.targetId);
    let foreignReadable = false;
    try {
      const attached = await pageAgent.call('Target.attachToTarget', { targetId: b.targetId, flatten: true });
      foreignReadable = (await evaluate(pageAgent, attached, 'document.title')) === 'Kaze fixture /session-b';
    } catch (error) { report.observations.pageEndpointAttachError = error.message; }
    report.observations.pageEndpoint = { foreignWindowListed: foreignFound, foreignWindowReadable: foreignReadable, isolated: !foreignReadable && !foreignFound };
  } catch (error) { report.observations.pageEndpoint = { browserCommandsRejected: true, error: error.message }; }
  pageAgent.close();

  const separate = await launchBrowser('separate-session-browser');
  const separatePage = await newPage(separate, `${origin}/separate-session`);
  assert.equal((await evaluate(separate.cdp, separatePage, 'document.cookie')).includes('kaze_probe'), false);
  const separateTargets = (await separate.cdp.call('Target.getTargets')).targetInfos;
  assert.ok(!separateTargets.some(t => t.targetId === a.targetId || t.targetId === b.targetId));
  let attachRejected = false;
  try { await separate.cdp.call('Target.attachToTarget', { targetId: a.targetId, flatten: true }); } catch { attachRejected = true; }
  assert.equal(attachRejected, true);
  report.observations.separateInstances = { cookiesSeparated: true, foreignTargetsAbsent: true, foreignTargetAttachRejected: true, startupMs: separate.startupMs };

  if (process.platform === 'darwin') {
    report.observations.browserProcesses = [];
    for (const browser of [shared, separate]) {
      const info = await browser.cdp.call('SystemInfo.getProcessInfo');
      const pids = info.processInfo.map(p => p.id);
      const rows = execFileSync('/bin/ps', ['-p', pids.join(','), '-o', 'rss='], { encoding: 'utf8' });
      report.observations.browserProcesses.push({ label: browser.label, processCount: pids.length, rssKiB: rows.trim().split('\n').reduce((sum, line) => sum + Number(line.trim()), 0), note: 'RSS sum includes shared memory more than once; not physical footprint' });
    }
  }
  agent.close();
  await stopBrowser(separate);
  await stopBrowser(shared);
}

function summarize(result) {
  return { isError: !!result.isError, text: (result.content || []).filter(c => c.type === 'text').map(c => c.text).join('\n').slice(0, 14000), images: (result.content || []).filter(c => c.type === 'image').map(c => ({ mimeType: c.mimeType, bytes: Buffer.from(c.data, 'base64').length })) };
}

async function mcpProbe(kind, origin) {
  const observation = { kind, toolCalls: [], launches: 0 };
  report.observations[kind] = observation;
  let browser;
  let launching;
  const launch = async () => {
    if (!browser || browser.closed) {
      launching ??= (async () => {
        const next = await launchBrowser(`${kind}-lazy`);
        await newPage(next, `${origin}/${kind}-initial`);
        browser = next; observation.launches++;
        return next;
      })();
      await launching;
      launching = undefined;
    }
    return browser.ws;
  };
  const relay = await new FocusGuardRelay(launch).start(); relays.add(relay);
  const args = kind === 'playwright'
    ? [path.join(directory, 'node_modules/@playwright/mcp/cli.js'), '--cdp-endpoint', relay.url, '--output-dir', artifacts, '--timeout-navigation', '8000', '--timeout-action', '5000', '--no-webmcp']
    : [path.join(directory, 'node_modules/chrome-devtools-mcp/build/src/bin/chrome-devtools-mcp.js'), '--wsEndpoint', relay.url, '--no-usage-statistics', '--no-performance-crux', '--no-source-maps'];
  const mcp = new McpPeer(process.execPath, args, artifacts); mcps.add(mcp);
  observation.initialize = await mcp.initialize();
  const { tools } = await mcp.call('tools/list');
  await fs.writeFile(path.join(artifacts, `${kind}-tools.json`), JSON.stringify(tools, null, 2));
  observation.tools = tools.map(t => t.name);
  observation.afterDiscovery = { relayConnections: relay.connections, browserLaunches: observation.launches, nodeProcessRunning: mcp.process.exitCode === null };
  assert.equal(relay.connections, 0, `${kind} connected before its first tool call`);
  assert.equal(observation.launches, 0, `${kind} launched a browser during discovery`);
  console.log(kind, 'after discovery', JSON.stringify(observation.afterDiscovery));
  if (process.platform === 'darwin') {
    await delay(150);
    const cpuBefore = execFileSync('/bin/ps', ['-p', String(mcp.process.pid), '-o', 'time=,rss='], { encoding: 'utf8' }).trim();
    await delay(1000);
    const cpuAfter = execFileSync('/bin/ps', ['-p', String(mcp.process.pid), '-o', 'time=,rss='], { encoding: 'utf8' }).trim();
    observation.idleWorker = { intervalMs: 1000, cpuTimeAndRssBefore: cpuBefore, cpuTimeAndRssAfter: cpuAfter, browserLaunches: observation.launches, relayConnections: relay.connections };
    assert.equal(observation.launches, 0);
  }
  const call = async (name, args) => {
    const start = performance.now();
    const blockedBefore = relay.blocked.length;
    const result = await mcp.call('tools/call', { name, arguments: args });
    const summary = summarize(result);
    observation.toolCalls.push({ name, args, ms: Math.round(performance.now() - start), focusCommandsBlocked: relay.blocked.slice(blockedBefore), ...summary });
    console.log(kind, name, JSON.stringify({ ms: observation.toolCalls.at(-1).ms, ...summary }).slice(0, 2400));
    return summary;
  };
  if (kind === 'playwright') {
    const navigation = await call('browser_navigate', { url: `${origin}/playwright` });
    assert.equal(navigation.isError, false);
    const snapshot = await call('browser_snapshot', {});
    const input = snapshot.text.match(/textbox "Message" \[ref=([^\]]+)\]/)?.[1];
    const button = snapshot.text.match(/button "Apply" \[ref=([^\]]+)\]/)?.[1];
    assert.ok(input && button, 'fixture controls absent from snapshot');
    assert.equal((await call('browser_type', { target: input, text: 'playwright-applied' })).isError, false);
    assert.equal((await call('browser_click', { target: button })).isError, false);
    assert.match((await call('browser_snapshot', {})).text, /playwright-applied/);
    assert.match((await call('browser_console_messages', { level: 'error' })).text, /kaze-fixture-console-error/);
    assert.match((await call('browser_network_requests', { includeStatic: true })).text, /network-failure.*FAILED/);
    const screenshot = await call('browser_take_screenshot', { type: 'png', scale: 'css' });
    assert.ok(screenshot.images.some(image => image.bytes > 100));
    await call('browser_tabs', { action: 'list' });
    await call('browser_tabs', { action: 'new', url: `${origin}/playwright-new-page` });
    await call('browser_tabs', { action: 'select', index: 0 });
  } else {
    const listed = await call('list_pages', {});
    const pageId = Number(listed.text.match(/(?:^|\n)(\d+): /)?.[1]);
    assert.ok(Number.isInteger(pageId));
    assert.equal((await call('navigate_page', { pageId, type: 'url', url: `${origin}/devtools` })).isError, false);
    const snapshot = await call('take_snapshot', { pageId });
    const input = snapshot.text.match(/uid=([^\s]+) textbox "Message"/)?.[1];
    const button = snapshot.text.match(/uid=([^\s]+) button "Apply"/)?.[1];
    assert.ok(input && button, 'fixture controls absent from DevTools snapshot');
    assert.equal((await call('fill', { pageId, uid: input, value: 'devtools-applied' })).isError, false);
    assert.equal((await call('click', { pageId, uid: button })).isError, false);
    assert.match((await call('take_snapshot', { pageId })).text, /devtools-applied/);
    assert.match((await call('list_console_messages', { pageId, types: ['error'] })).text, /kaze-fixture-console-error/);
    assert.match((await call('list_network_requests', { pageId })).text, /network-failure/);
    const screenshot = await call('take_screenshot', { pageId, format: 'png' });
    assert.ok(screenshot.images.some(image => image.bytes > 100));
    await call('select_page', { pageId, bringToFront: true });
  }
  const foreign = await newPage(browser, `${origin}/foreign-window`);
  const foreignContext = await browser.cdp.call('Target.createBrowserContext');
  const foreignIsolated = await newPage(browser, `${origin}/foreign-context`, foreignContext.browserContextId);
  const allPages = kind === 'playwright' ? await call('browser_tabs', { action: 'list' }) : await call('list_pages', {});
  observation.unfilteredRelay = { foreignWindowListed: allPages.text.includes('/foreign-window'), foreignContextListed: allPages.text.includes('/foreign-context') };
  assert.equal(observation.unfilteredRelay.foreignWindowListed, true);
  await browser.cdp.call('Target.closeTarget', { targetId: foreign.targetId });
  await browser.cdp.call('Target.closeTarget', { targetId: foreignIsolated.targetId });
  await browser.cdp.call('Target.disposeBrowserContext', { browserContextId: foreignContext.browserContextId });

  const neighbor = await launchBrowser(`${kind}-neighbor`);
  await newPage(neighbor, `${origin}/neighbor-session`);
  const scopedPages = kind === 'playwright' ? await call('browser_tabs', { action: 'list' }) : await call('list_pages', {});
  observation.separateInstanceNeighborHidden = !scopedPages.text.includes('/neighbor-session');
  assert.equal(observation.separateInstanceNeighborHidden, true);
  await stopBrowser(neighbor);
  const beforeRestart = observation.launches;
  await stopBrowser(browser);
  await delay(200);
  const firstReconnect = kind === 'playwright'
    ? await call('browser_navigate', { url: `${origin}/playwright-reconnected` })
    : await call('list_pages', {});
  observation.reconnect = { firstCallSucceeded: !firstReconnect.isError, browserRelaunched: observation.launches > beforeRestart };
  if (firstReconnect.isError) {
    const retry = kind === 'playwright'
      ? await call('browser_navigate', { url: `${origin}/playwright-reconnected` })
      : await call('list_pages', {});
    observation.reconnect.retrySucceeded = !retry.isError;
    observation.reconnect.browserRelaunched = observation.launches > beforeRestart;
  }
  observation.connections = relay.connections;
  observation.focusCommandsBlocked = relay.blocked;
  observation.cdpMethods = [...new Set(relay.commands)];
  await mcp.close(); mcps.delete(mcp);
  if (browser) await stopBrowser(browser);
  await relay.close(); relays.delete(relay);
}

try {
  if (process.platform === 'darwin') {
    monitor = spawn(path.join(artifacts, 'focus-monitor'), [], { stdio: ['ignore', 'pipe', 'pipe'] });
    let buffer = '';
    monitor.stdout.on('data', chunk => {
      buffer += chunk;
      for (;;) { const end = buffer.indexOf('\n'); if (end < 0) break; focusEvents.push(JSON.parse(buffer.slice(0, end))); buffer = buffer.slice(end + 1); }
    });
    await delay(150);
    assert.ok(focusEvents.length > 0, 'native focus observer did not report its initial state');
  }
  fixture = createServer((request, response) => {
    if (request.url.startsWith('/network-failure')) { request.socket.destroy(); return; }
    if (request.url === '/favicon.ico') { response.writeHead(204); response.end(); return; }
    response.writeHead(200, { 'Content-Type': 'text/html', 'Cache-Control': 'no-store' });
    response.end(`<!doctype html><title>Kaze fixture ${request.url}</title><h1>Kaze browser fixture</h1><label>Message <input id="message"></label><button id="apply">Apply</button><p id="result" role="status">Waiting</p><script>document.querySelector('#apply').onclick=()=>document.querySelector('#result').textContent=document.querySelector('#message').value||'Applied';console.error('kaze-fixture-console-error');fetch('/network-failure').catch(()=>{});</script>`);
  });
  fixture.listen(0, '127.0.0.1'); await once(fixture, 'listening');
  const origin = `http://127.0.0.1:${fixture.address().port}`;
  report.fixtureOrigin = origin;
  for (const phase of phases) {
    console.log('PHASE', phase);
    if (phase === 'isolation') await isolation(origin);
    else for (const kind of ['playwright', 'devtools']) await mcpProbe(kind, origin);
  }
} catch (error) {
  report.error = error.stack;
  console.error(error);
  process.exitCode = 1;
} finally {
  for (const mcp of mcps) await mcp.close().catch(() => {});
  for (const relay of relays) await relay.close().catch(() => {});
  for (const browser of browsers) await stopBrowser(browser).catch(() => {});
  if (fixture) await new Promise(resolve => fixture.close(resolve));
  if (monitor) {
    report.focusMonitor = { baselineReceived: focusEvents.length > 0, runningUntilCleanup: monitor.exitCode === null };
    if (!report.focusMonitor.runningUntilCleanup) { report.error ??= 'native focus observer exited before cleanup'; process.exitCode = 1; }
    monitor.kill('SIGTERM'); await delay(100);
  }
  report.focusEvents = focusEvents;
  const ownedPids = new Set(report.browserInstances.map(browser => browser.pid));
  report.chromeActivationEvents = focusEvents.filter(event => ownedPids.has(event.pid));
  if (report.chromeActivationEvents.length) { report.error ??= 'test Chrome became the foreground application'; process.exitCode = 1; }
  await fs.writeFile(path.join(artifacts, `${phases.join('-')}-results.json`), JSON.stringify(report, null, 2) + '\n');
  console.log('RESULT', JSON.stringify(report.observations, null, 2));
  console.log('Chrome activation events:', report.chromeActivationEvents.length);
}
