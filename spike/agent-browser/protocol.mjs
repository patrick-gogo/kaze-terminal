import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { createServer } from 'node:http';
import { WebSocket, WebSocketServer } from 'ws';

export const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

// This is a focus guard for trusted localhost fixtures, NOT session isolation.
// It intentionally acknowledges explicit activation requests without performing them.
export function guardCommand(command) {
  if (['Page.bringToFront', 'Target.activateTarget', 'Browser.setWindowBounds', 'Target.sendMessageToTarget'].includes(command.method)) {
    return { blocked: true };
  }
  if (command.method === 'Target.createTarget') {
    return { command: { ...command, params: { ...command.params, background: true } } };
  }
  return { command };
}

export class CdpPeer {
  constructor(socket) {
    this.socket = socket;
    this.next = 0;
    this.pending = new Map();
    this.events = [];
    socket.on('message', raw => {
      const message = JSON.parse(raw);
      if (!message.id) { this.events.push(message); return; }
      const pending = this.pending.get(message.id);
      if (!pending) return;
      clearTimeout(pending.timer);
      this.pending.delete(message.id);
      if (message.error) pending.reject(new Error(JSON.stringify(message.error)));
      else pending.resolve(message.result);
    });
    socket.on('error', () => {});
    socket.on('close', () => {
      for (const pending of this.pending.values()) {
        clearTimeout(pending.timer);
        pending.reject(new Error('CDP disconnected'));
      }
      this.pending.clear();
    });
  }
  static async connect(url) {
    const socket = new WebSocket(url, { handshakeTimeout: 5000 });
    await once(socket, 'open');
    return new CdpPeer(socket);
  }
  call(method, params = {}, sessionId) {
    const id = ++this.next;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error(`CDP timeout: ${method}`)); }, 8000);
      this.pending.set(id, { resolve, reject, timer });
      this.socket.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    });
  }
  close() { this.socket.close(); }
}

export class FocusGuardRelay {
  constructor(getEndpoint) {
    this.getEndpoint = getEndpoint;
    this.connections = 0;
    this.commands = [];
    this.blocked = [];
    this.sockets = new Set();
  }
  async start() {
    this.server = createServer((_, response) => { response.writeHead(404); response.end(); });
    this.wss = new WebSocketServer({ server: this.server });
    this.wss.on('connection', async client => {
      this.connections++;
      this.sockets.add(client);
      const queue = [];
      let upstream;
      client.on('error', () => {});
      client.on('close', () => { this.sockets.delete(client); upstream?.close(); });
      client.on('message', raw => {
        const message = JSON.parse(raw);
        this.commands.push(message.method);
        const guarded = guardCommand(message);
        if (guarded.blocked) {
          this.blocked.push(message.method);
          client.send(JSON.stringify({ id: message.id, result: {}, ...(message.sessionId ? { sessionId: message.sessionId } : {}) }));
          return;
        }
        const data = JSON.stringify(guarded.command);
        if (upstream?.readyState === WebSocket.OPEN) upstream.send(data);
        else queue.push(data);
      });
      try {
        const url = await this.getEndpoint();
        if (client.readyState !== WebSocket.OPEN) return;
        upstream = new WebSocket(url, { handshakeTimeout: 5000 });
        this.sockets.add(upstream);
        upstream.on('error', () => client.close());
        upstream.on('close', () => { this.sockets.delete(upstream); client.close(); });
        upstream.on('message', raw => { if (client.readyState === WebSocket.OPEN) client.send(raw); });
        await once(upstream, 'open');
        for (const data of queue) upstream.send(data);
      } catch { client.close(); }
    });
    this.server.listen(0, '127.0.0.1');
    await once(this.server, 'listening');
    this.url = `ws://127.0.0.1:${this.server.address().port}`;
    return this;
  }
  async close() {
    for (const socket of this.sockets) socket.terminate();
    this.wss.close();
    await new Promise(resolve => this.server.close(resolve));
  }
}

export class McpPeer {
  constructor(command, args, cwd) {
    this.next = 0;
    this.pending = new Map();
    this.stderr = '';
    this.buffer = '';
    this.process = spawn(command, args, {
      cwd, stdio: ['pipe', 'pipe', 'pipe'],
      env: { ...process.env, CHROME_DEVTOOLS_MCP_NO_USAGE_STATISTICS: '1', PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD: '1', PUPPETEER_SKIP_DOWNLOAD: '1' }
    });
    this.process.stdout.setEncoding('utf8');
    this.process.stdout.on('data', data => {
      this.buffer += data;
      if (this.buffer.length > 16000000) { this.process.kill(); return; }
      for (;;) {
        const end = this.buffer.indexOf('\n');
        if (end < 0) break;
        const line = this.buffer.slice(0, end); this.buffer = this.buffer.slice(end + 1);
        if (!line.trim()) continue;
        let message;
        try { message = JSON.parse(line); } catch { continue; }
        if (message.method && message.id !== undefined) {
          const result = message.method === 'roots/list' ? { roots: [{ uri: new URL(`file://${cwd}/`).href, name: 'Kaze localhost fixture' }] } : {};
          this.send({ jsonrpc: '2.0', id: message.id, result });
          continue;
        }
        const pending = this.pending.get(message.id);
        if (!pending) continue;
        clearTimeout(pending.timer); this.pending.delete(message.id);
        if (message.error) pending.reject(new Error(JSON.stringify(message.error)));
        else pending.resolve(message.result);
      }
    });
    this.process.stderr.on('data', data => { this.stderr = (this.stderr + data).slice(-32768); });
    this.process.on('error', error => this.fail(error));
    this.process.on('exit', (code, signal) => this.fail(new Error(`MCP exited ${code ?? signal}: ${this.stderr.slice(-2000)}`)));
  }
  fail(error) {
    for (const pending of this.pending.values()) { clearTimeout(pending.timer); pending.reject(error); }
    this.pending.clear();
  }
  send(message) { this.process.stdin.write(JSON.stringify(message) + '\n'); }
  call(method, params = {}) {
    const id = ++this.next;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error(`MCP timeout: ${method}: ${this.stderr.slice(-1000)}`)); }, 20000);
      this.pending.set(id, { resolve, reject, timer });
      this.send({ jsonrpc: '2.0', id, method, params });
    });
  }
  async initialize() {
    const result = await this.call('initialize', { protocolVersion: '2024-11-05', capabilities: { roots: { listChanged: false } }, clientInfo: { name: 'kaze-browser-spike', version: '0.1.0' } });
    this.send({ jsonrpc: '2.0', method: 'notifications/initialized' });
    return result;
  }
  async close() {
    this.process.stdin.end();
    await Promise.race([once(this.process, 'exit'), delay(1500)]);
    if (this.process.exitCode === null && !this.process.killed) this.process.kill('SIGTERM');
  }
}
