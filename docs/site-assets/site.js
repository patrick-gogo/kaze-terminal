const tabs = [...document.querySelectorAll('.platform-tabs [role="tab"]')];
function selectPlatform(platform, focus = false) {
  tabs.forEach(tab => {
    const selected = tab.id === `tab-${platform}`;
    tab.setAttribute('aria-selected', String(selected));
    tab.tabIndex = selected ? 0 : -1;
    document.getElementById(tab.getAttribute('aria-controls')).hidden = !selected;
    if (selected && focus) tab.focus();
  });
}
tabs.forEach((tab, index) => {
  tab.addEventListener('click', () => selectPlatform(tab.id.slice(4)));
  tab.addEventListener('keydown', event => {
    let next;
    if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') next = (index + 1) % tabs.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = tabs.length - 1;
    else return;
    event.preventDefault();
    selectPlatform(tabs[next].id.slice(4), true);
  });
});
document.querySelectorAll('[data-platform]').forEach(link => {
  link.addEventListener('click', () => selectPlatform(link.dataset.platform));
});
document.querySelectorAll('[data-copy]').forEach(button => {
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(button.dataset.copy);
      button.textContent = 'Copied';
      document.getElementById('copy-status').textContent = 'Commands copied to clipboard.';
      setTimeout(() => { button.textContent = 'Copy'; }, 1800);
    } catch {
      button.textContent = 'Select text';
      document.getElementById('copy-status').textContent = 'Copy unavailable. Select and copy the commands below.';
    }
  });
});

const captures = {
  sessions: { alt: 'Kaze on macOS: a sidebar of sessions marked needs you, running and done, with an API server\'s log floating over the todo-api session', caption: 'The session sidebar with a server floating over a session' },
  agents: { alt: 'Kaze with three Claude Code subagents listed under the TODO-142 session in the sidebar, and a live pane showing a finished subagent\'s steps and report', caption: 'Subagents under their session, with a live pane' },
  board: { alt: 'The Kaze agents board with running and done subagents from three sessions, each card naming its task, session and current step', caption: 'The agents board, opened with Cmd+Shift+A' },
  palette: { alt: 'The Kaze palette listing a waiting session, every session and server, and demo skills', caption: 'The palette, opened with Cmd+K' },
  scratch: { alt: 'A scratch shell floating over the todo-api session, showing git status and git diff', caption: 'A scratch shell in the session\'s folder' },
  workspaces: { alt: 'The Kaze Workspaces settings page with a card for the running three-member Todo API demo workspace', caption: 'The workspace card with API, Web and Health running' },
  settings: { alt: 'Kaze Appearance settings with five theme cards, Midnight selected, and a live terminal preview', caption: 'Appearance settings with theme cards and a live preview' },
  about: { alt: 'The Kaze About page showing the logo, version 0.15 and platform, chip and license details', caption: 'About Kaze 0.15 on macOS' }
};
function showShot(shot) {
  const image = document.getElementById('product-shot');
  image.src = `site-assets/kaze-${shot}.png`;
  image.alt = captures[shot].alt;
  document.getElementById('capture-description').textContent = captures[shot].caption;
  document.querySelectorAll('[data-shot]').forEach(item => item.setAttribute('aria-pressed', String(item.dataset.shot === shot)));
}
document.querySelectorAll('[data-shot]').forEach(button => {
  button.addEventListener('click', () => showShot(button.dataset.shot));
});
document.querySelectorAll('[data-show]').forEach(button => {
  button.addEventListener('click', () => {
    showShot(button.dataset.show);
    document.getElementById('product').scrollIntoView({ block: 'start' });
  });
});

// Windows visitors get the Windows download first and the Windows steps open.
if (/Windows/.test(navigator.userAgent)) {
  const mac = document.querySelector('[data-download="mac"]');
  const win = document.querySelector('[data-download="windows"]');
  if (mac && win) {
    mac.classList.replace('primary', 'secondary');
    win.classList.replace('secondary', 'primary');
    win.parentNode.insertBefore(win, mac);
  }
  selectPlatform('windows');
}

// The download count: every release zip on GitHub, summed. Kaze's in-place
// updates download the same zips, so they count too. Hidden until there is one.
fetch('https://api.github.com/repos/patrick-gogo/kaze-terminal/releases?per_page=100')
  .then(r => (r.ok ? r.json() : []))
  .then(releases => {
    const total = releases
      .flatMap(r => r.assets || [])
      .filter(a => a.name.endsWith('.zip'))
      .reduce((n, a) => n + a.download_count, 0);
    if (total > 0) {
      const count = document.getElementById('download-count');
      count.textContent = `${total.toLocaleString('en-US')} ${total === 1 ? 'download' : 'downloads'}`;
      count.hidden = false;
      document.getElementById('download-sep').hidden = false;
    }
  })
  .catch(() => {});
