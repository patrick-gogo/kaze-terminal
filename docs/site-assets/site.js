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
  terminal: { alt: 'Kaze 0.6 running on macOS with a four-tab workspace menu and real Go test output', caption: 'Workspace tabs on macOS' },
  workspaces: { alt: 'The real Kaze Workspaces settings page with a saved four-tab demo recipe and startup selection', caption: 'Workspaces settings on macOS' },
  about: { alt: 'The real Kaze About page showing the logo, version 0.6, build details, and copy button', caption: 'About Kaze 0.6 on macOS' }
};
document.querySelectorAll('[data-shot]').forEach(button => {
  button.addEventListener('click', () => {
    const shot = button.dataset.shot;
    const image = document.getElementById('product-shot');
    image.src = `site-assets/kaze-${shot}.png`;
    image.alt = captures[shot].alt;
    document.getElementById('capture-description').textContent = captures[shot].caption;
    document.querySelectorAll('[data-shot]').forEach(item => item.setAttribute('aria-pressed', String(item === button)));
  });
});
