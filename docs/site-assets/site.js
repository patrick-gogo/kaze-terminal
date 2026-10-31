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
  palette: { alt: 'The Kaze palette listing a waiting session, every session and server, and Claude Code skills', caption: 'The palette, opened with Cmd+K' },
  scratch: { alt: 'A scratch shell floating over the todo-api session, showing git status and git diff', caption: 'A scratch shell in the session\'s folder' },
  workspaces: { alt: 'The Kaze Workspaces settings page with a running three-tab demo workspace', caption: 'Workspaces settings on macOS' },
  about: { alt: 'The Kaze About page showing the logo, version 0.10 and build details', caption: 'About Kaze 0.10 on macOS' }
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
