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
  const fullCapture = document.getElementById('full-capture');
  if (fullCapture) fullCapture.href = image.src;
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

// Rotate real product captures while the gallery is visible and idle.
(() => {
  const gallery = document.getElementById('product');
  if (!gallery) return;
  const image = document.getElementById('product-shot');
  const choices = [...gallery.querySelectorAll('[data-shot]')];
  const description = document.getElementById('capture-description');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  let playing = !reduced.matches, visible = false, hovering = false, timer;
  let advancing = false;
  const index = () => choices.findIndex(button => button.getAttribute('aria-pressed') === 'true');
  const canRotate = () => playing && visible && !hovering && !document.hidden && !gallery.contains(document.activeElement);
  function schedule() {
    clearTimeout(timer);
    description.setAttribute('aria-live', playing ? 'off' : 'polite');
    if (canRotate()) {
      timer = setTimeout(() => {
        if (canRotate()) advance(1);
        else schedule();
      }, 2500);
    }
  }
  function advance(direction) {
    advancing = true;
    choices[(index() + direction + choices.length) % choices.length].click();
    advancing = false;
    schedule();
  }
  choices.forEach(button => button.addEventListener('click', () => {
    if (!advancing) playing = false;
    document.getElementById('full-capture').href = image.src;
    schedule();
  }));
  document.querySelectorAll('[data-show]').forEach(button => button.addEventListener('click', () => {
    playing = false;
    schedule();
  }));
  gallery.addEventListener('pointerenter', event => { if (event.pointerType === 'mouse') { hovering = true; schedule(); } });
  gallery.addEventListener('pointerleave', () => { hovering = false; schedule(); });
  gallery.addEventListener('focusin', schedule);
  gallery.addEventListener('focusout', () => setTimeout(schedule, 0));
  document.addEventListener('visibilitychange', schedule);
  reduced.addEventListener('change', () => { if (reduced.matches) playing = false; schedule(); });
  new IntersectionObserver(entries => { visible = entries[0].isIntersecting; schedule(); }, { threshold: 0.25 }).observe(image);
  image.addEventListener('load', () => {
    if (!reduced.matches) {
      image.getAnimations().forEach(animation => animation.cancel());
      image.animate([{ opacity: 0.6 }, { opacity: 1 }], { duration: 220, easing: 'ease-out' });
    }
  });
  schedule();
})();
