const tabs = [...document.querySelectorAll('[role="tab"]')];
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
