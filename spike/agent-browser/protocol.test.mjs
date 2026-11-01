import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { WebSocketServer } from 'ws';
import { guardCommand, FocusGuardRelay, CdpPeer } from './protocol.mjs';

test('explicit focus commands never reach the test browser', () => {
  for (const method of ['Page.bringToFront', 'Target.activateTarget', 'Browser.setWindowBounds']) {
    assert.equal(guardCommand({ id: 1, method }).blocked, true);
  }
});

test('new targets are always created in the background', () => {
  const command = { id: 2, method: 'Target.createTarget', params: { url: 'about:blank', background: false } };
  const result = guardCommand(command);
  assert.equal(result.command.params.background, true);
  assert.equal(command.params.background, false);
});

test('ordinary commands and their routing remain unchanged', () => {
  const command = { id: 3, sessionId: 'owned', method: 'Page.captureScreenshot', params: { format: 'png' } };
  assert.deepEqual(guardCommand(command), { command });
});

test('legacy tunneled commands cannot bypass the focus guard', () => {
  assert.equal(guardCommand({ id: 4, method: 'Target.sendMessageToTarget' }).blocked, true);
});

test('the real relay launches lazily and does not forward focus requests', { timeout: 4000 }, async () => {
  const upstream = new WebSocketServer({ port: 0, host: '127.0.0.1' });
  await once(upstream, 'listening');
  const received = [];
  upstream.on('connection', socket => socket.on('message', data => {
    const command = JSON.parse(data);
    received.push(command);
    socket.send(JSON.stringify({ id: command.id, result: { targetId: 'fixture' } }));
  }));
  let starts = 0;
  const relay = await new FocusGuardRelay(async () => {
    starts++;
    return `ws://127.0.0.1:${upstream.address().port}`;
  }).start();
  let client;
  try {
    assert.equal(starts, 0);
    client = await CdpPeer.connect(relay.url);
    assert.deepEqual(await client.call('Page.bringToFront'), {});
    await client.call('Target.createTarget', { url: 'about:blank', background: false });
    assert.equal(starts, 1);
    assert.equal(received.length, 1);
    assert.equal(received[0].params.background, true);
    assert.deepEqual(relay.blocked, ['Page.bringToFront']);
  } finally {
    client?.close();
    await relay.close();
    for (const socket of upstream.clients) socket.terminate();
    await new Promise(resolve => upstream.close(resolve));
  }
});
