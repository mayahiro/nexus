package chromium

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

const startupTimeout = 5 * time.Second
const shutdownTimeout = 5 * time.Second
const maxLogEntries = 200
const pageTargetTimeout = 5 * time.Second
const defaultViewportWidth = 1920
const defaultViewportHeight = 1080

const observeTreeJS = `(function () {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const roleFor = (el) => {
    const ariaRole = (el.getAttribute('role') || '').trim();
    if (ariaRole) return ariaRole;

    const tag = el.tagName.toLowerCase();
    if (tag === 'a') return 'link';
    if (tag === 'button') return 'button';
    if (tag === 'textarea') return 'textbox';
    if (tag === 'select') return 'combobox';
    if (tag === 'summary') return 'button';
    if (tag === 'input') {
      const type = (el.getAttribute('type') || 'text').toLowerCase();
      if (type === 'checkbox') return 'checkbox';
      if (type === 'radio') return 'radio';
      if (type === 'submit' || type === 'button' || type === 'reset') return 'button';
      return 'textbox';
    }
    if (el.isContentEditable) return 'textbox';
    return tag;
  };

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const valueFor = (el) => {
    if ('value' in el && typeof el.value === 'string') return el.value.trim();
    return '';
  };

  const nameFor = (el) => {
    const label = (el.getAttribute('aria-label') || '').trim();
    if (label) return label;
    const labelledby = (el.getAttribute('aria-labelledby') || '').trim();
    if (labelledby) {
      const text = labelledby
        .split(/\s+/)
        .map((id) => document.getElementById(id))
        .filter(Boolean)
        .map((node) => (node.innerText || node.textContent || '').trim())
        .join(' ')
        .trim();
      if (text) return text;
    }
    const text = (el.innerText || el.textContent || '').trim();
    if (text) return text;
    const value = valueFor(el);
    if (value) return value;
    return '';
  };

  const textFor = (el) => {
    return (el.innerText || el.textContent || '').trim();
  };

  const attrsFor = (el) => {
    const attrs = {};
    attrs.tag = el.tagName.toLowerCase();
    if (el.id) attrs.id = el.id;
    if (el.getAttribute('name')) attrs.name = el.getAttribute('name');
    if (el.getAttribute('type')) attrs.type = el.getAttribute('type');
    if (el.getAttribute('href')) attrs.href = el.getAttribute('href');
    if (el.getAttribute('placeholder')) attrs.placeholder = el.getAttribute('placeholder');
    if (el.getAttribute('aria-label')) attrs['aria-label'] = el.getAttribute('aria-label');
    if (el.getAttribute('aria-labelledby')) attrs['aria-labelledby'] = el.getAttribute('aria-labelledby');
    if (el.getAttribute('data-testid')) attrs['data-testid'] = el.getAttribute('data-testid');
    if (el.getAttribute('data-test')) attrs['data-test'] = el.getAttribute('data-test');
    return attrs;
  };

  const normalize = (value) => (value || '').trim().replace(/\s+/g, ' ').slice(0, 80);

  const fingerprintFor = (el, role, name, attrs) => {
    const parts = [
      attrs.tag || el.tagName.toLowerCase(),
      role || '',
      attrs.id || '',
      attrs.name || '',
      attrs['data-testid'] || attrs['data-test'] || '',
      attrs['aria-label'] || '',
      attrs.href || '',
      attrs.placeholder || '',
      normalize(name),
      normalize(el.innerText || el.textContent || '')
    ];
    return parts.join('|');
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const ids = new Map();
  candidates.forEach((el, index) => ids.set(el, index + 1));

  const nodes = candidates.map((el, index) => {
    const rect = el.getBoundingClientRect();
    const parentCandidate = candidates.find((candidate) => candidate !== el && candidate.contains(el) && !Array.from(candidates).some((other) => other !== candidate && other !== el && other.contains(el) && candidate.contains(other)));
    const children = candidates.filter((candidate) => candidate !== el && el.contains(candidate) && !Array.from(candidates).some((other) => other !== candidate && other !== el && el.contains(other) && other.contains(candidate))).map((child) => ids.get(child));

    const role = roleFor(el);
    const name = nameFor(el);
    const attrs = attrsFor(el);

    return {
      id: index + 1,
      fingerprint: fingerprintFor(el, role, name, attrs),
      role: role,
      name: name,
      text: textFor(el),
      value: valueFor(el),
      bounds: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        w: Math.round(rect.width),
        h: Math.round(rect.height)
      },
      visible: true,
      enabled: !el.disabled && el.getAttribute('aria-disabled') !== 'true',
      focused: document.activeElement === el,
      editable: el.isContentEditable || el.tagName === 'INPUT' || el.tagName === 'TEXTAREA',
      selectable: el.tagName === 'SELECT' || el.getAttribute('role') === 'tab' || el.getAttribute('type') === 'checkbox' || el.getAttribute('type') === 'radio',
      invokable: el.tagName === 'BUTTON' || el.tagName === 'A' || !!el.onclick || el.getAttribute('role') === 'button' || el.getAttribute('role') === 'link',
      scrollable: el.scrollHeight > el.clientHeight || el.scrollWidth > el.clientWidth,
      children,
      attrs: attrs,
      parent_id: parentCandidate ? ids.get(parentCandidate) : null
    };
  });

  return JSON.stringify(nodes);
})()`

const clickNodeJS = `(function (nodeID) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = candidates[nodeID - 1];
  if (!el) {
    throw new Error('node not found');
  }

  if (el.disabled || el.getAttribute('aria-disabled') === 'true') {
    throw new Error('node is disabled');
  }

  el.scrollIntoView({block: 'center', inline: 'center'});
  el.focus();
  el.click();

  return {
    id: nodeID,
    tag: el.tagName.toLowerCase(),
    text: (el.innerText || el.textContent || '').trim()
  };
})($NODE_ID$)`

const typeNodeJS = `(function (nodeID, text) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = nodeID > 0 ? candidates[nodeID - 1] : document.activeElement;
  if (!el) {
    throw new Error('editable node not found');
  }

  const tag = el.tagName.toLowerCase();
  const editable = el.isContentEditable || tag === 'input' || tag === 'textarea';
  if (!editable) {
    throw new Error('node is not editable');
  }

  if (el.disabled || el.getAttribute('aria-disabled') === 'true') {
    throw new Error('node is disabled');
  }

  el.scrollIntoView({block: 'center', inline: 'center'});
  el.focus();

  if (tag === 'input' || tag === 'textarea') {
    el.value = text;
    if (typeof el.setSelectionRange === 'function') {
      el.setSelectionRange(text.length, text.length);
    }
  } else {
    el.textContent = text;
  }

  el.dispatchEvent(new Event('input', {bubbles: true}));
  el.dispatchEvent(new Event('change', {bubbles: true}));

  return {
    id: nodeID > 0 ? nodeID : null,
    tag,
    text: text
  };
})($NODE_ID$, $TEXT$)`

const scrollJS = `(function (nodeID, dir, amount) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const deltaFor = (base) => amount > 0 ? amount : Math.max(100, Math.round(base * 0.8));
  const sign = dir === 'up' ? -1 : 1;

  if (nodeID > 0) {
    const candidates = Array.from(document.querySelectorAll(selector))
      .filter((el) => visible(el));
    const el = candidates[nodeID - 1];
    if (!el) {
      throw new Error('node not found');
    }
    const delta = sign * deltaFor(el.clientHeight || window.innerHeight);
    el.scrollTop += delta;
    return {
      scope: 'node',
      id: nodeID,
      dir,
      amount: Math.abs(delta),
      top: el.scrollTop
    };
  }

  const delta = sign * deltaFor(window.innerHeight);
  window.scrollBy(0, delta);
  return {
    scope: 'viewport',
    dir,
    amount: Math.abs(delta),
    x: window.scrollX,
    y: window.scrollY
  };
})($NODE_ID$, $DIR$, $AMOUNT$)`

const getNodeJS = `(function (kind, nodeID) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = candidates[nodeID - 1];
  if (!el) {
    throw new Error('node not found');
  }

  switch (kind) {
    case 'text':
      return (el.innerText || el.textContent || '').trim();
    case 'value':
      if ('value' in el && typeof el.value === 'string') {
        return el.value;
      }
      return (el.textContent || '').trim();
    case 'attributes':
      return Object.fromEntries(Array.from(el.attributes).map((attr) => [attr.name, attr.value]));
    case 'bbox': {
      const rect = el.getBoundingClientRect();
      return {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        width: Math.round(rect.width),
        height: Math.round(rect.height)
      };
    }
    default:
      throw new Error('unsupported get kind');
  }
})($KIND$, $NODE_ID$)`

const selectNodeJS = `(function (nodeID, value) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = candidates[nodeID - 1];
  if (!el) {
    throw new Error('node not found');
  }
  if (el.tagName !== 'SELECT') {
    throw new Error('node is not a select');
  }

  const option = Array.from(el.options).find((opt) => opt.value === value || (opt.textContent || '').trim() === value);
  if (!option) {
    throw new Error('option not found');
  }

  el.value = option.value;
  el.dispatchEvent(new Event('input', {bubbles: true}));
  el.dispatchEvent(new Event('change', {bubbles: true}));

  return {
    id: nodeID,
    value: el.value,
    label: (option.textContent || '').trim()
  };
})($NODE_ID$, $VALUE$)`

const markUploadNodeJS = `(function (nodeID, token) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = candidates[nodeID - 1];
  if (!el) {
    throw new Error('node not found');
  }
  if (el.tagName !== 'INPUT' || (el.getAttribute('type') || '').toLowerCase() !== 'file') {
    throw new Error('node is not a file input');
  }

  el.setAttribute('data-nexus-upload', token);
  return {
    id: nodeID,
    selector: '[data-nexus-upload="' + token + '"]'
  };
})($NODE_ID$, $TOKEN$)`

const markTypeTargetJS = `(function (nodeID, token) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = nodeID > 0 ? candidates[nodeID - 1] : document.activeElement;
  if (!el) {
    throw new Error('editable node not found');
  }

  const tag = el.tagName.toLowerCase();
  const editable = el.isContentEditable || tag === 'input' || tag === 'textarea';
  if (!editable) {
    throw new Error('node is not editable');
  }

  if (el.disabled || el.getAttribute('aria-disabled') === 'true') {
    throw new Error('node is disabled');
  }

  el.scrollIntoView({block: 'center', inline: 'center'});
  el.focus();

  if ((tag === 'input' || tag === 'textarea') && typeof el.setSelectionRange === 'function' && typeof el.value === 'string') {
    el.setSelectionRange(el.value.length, el.value.length);
  }

  el.setAttribute('data-nexus-type', token);

  return {
    id: nodeID > 0 ? nodeID : null,
    tag,
    selector: '[data-nexus-type="' + token + '"]'
  };
})($NODE_ID$, $TOKEN$)`

const clearMarkedTypeTargetJS = `(function (token) {
  const el = document.querySelector('[data-nexus-type="' + token + '"]');
  if (el) {
    el.removeAttribute('data-nexus-type');
  }
  return true;
})($TOKEN$)`

const nodePointJS = `(function (nodeID) {
  const selector = [
    'button',
    'a[href]',
    'input',
    'textarea',
    'select',
    '[role="button"]',
    '[role="link"]',
    '[role="tab"]',
    '[role="checkbox"]',
    '[role="radio"]',
    '[contenteditable="true"]',
    '[contenteditable=""]',
    '[onclick]',
    '[tabindex]'
  ].join(',');

  const visible = (el) => {
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden') return false;
    if (el.hidden) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  };

  const candidates = Array.from(document.querySelectorAll(selector))
    .filter((el) => visible(el));

  const el = candidates[nodeID - 1];
  if (!el) {
    throw new Error('node not found');
  }

  el.scrollIntoView({block: 'center', inline: 'center'});
  const rect = el.getBoundingClientRect();
  return {
    id: nodeID,
    tag: el.tagName.toLowerCase(),
    x: rect.left + rect.width / 2,
    y: rect.top + rect.height / 2
  };
})($NODE_ID$)`

type nodePoint struct {
	ID  int     `json:"id"`
	Tag string  `json:"tag"`
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
}

type typeTarget struct {
	ID       int    `json:"id"`
	Tag      string `json:"tag"`
	Selector string `json:"selector"`
}

type Backend struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	waitCh      chan error
	userDataDir string
	devtoolsURL string
	logs        []api.LogEntry
}

var errPageTargetNotFound = errors.New("page target not found")

func New() *Backend {
	return &Backend{}
}

func (*Backend) Name() spec.BackendName {
	return spec.BackendChromium
}

func (*Backend) Capabilities() spec.Capabilities {
	return spec.Capabilities{
		Observe:    true,
		Act:        true,
		Screenshot: true,
		Logs:       true,
	}
}

func (b *Backend) Attach(_ context.Context, cfg spec.SessionConfig) error {
	if cfg.TargetRef == "" {
		return errors.New("chromium executable path is required")
	}

	if _, err := os.Stat(cfg.TargetRef); err != nil {
		return err
	}

	b.mu.Lock()
	if b.cmd != nil {
		b.mu.Unlock()
		return errors.New("chromium backend is already attached")
	}
	b.mu.Unlock()

	userDataDir, err := os.MkdirTemp("", "nexus-chromium-"+sanitize(cfg.SessionID)+"-")
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	args := []string{
		"--headless",
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		fmt.Sprintf("--window-size=%d,%d", viewportWidth(cfg.Options), viewportHeight(cfg.Options)),
		"--user-data-dir=" + userDataDir,
		initialURL(cfg.Options),
	}

	cmd := exec.CommandContext(runCtx, cfg.TargetRef, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		os.RemoveAll(userDataDir)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		os.RemoveAll(userDataDir)
		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(userDataDir)
		return err
	}

	waitCh := make(chan error, 1)
	startedCh := make(chan string, 1)

	b.mu.Lock()
	b.cmd = cmd
	b.cancel = cancel
	b.waitCh = waitCh
	b.userDataDir = userDataDir
	b.devtoolsURL = ""
	b.logs = nil
	b.mu.Unlock()

	go b.captureLogs(stdout, startedCh)
	go b.captureLogs(stderr, startedCh)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	timer := time.NewTimer(startupTimeout)
	defer timer.Stop()

	select {
	case url := <-startedCh:
		b.mu.Lock()
		b.devtoolsURL = url
		b.mu.Unlock()
		return nil
	case err := <-waitCh:
		b.cleanupAfterExit()
		if err == nil {
			return errors.New("chromium exited before startup completed")
		}
		return err
	case <-timer.C:
		if err := b.Detach(context.Background()); err != nil {
			return err
		}
		return errors.New("chromium startup timed out")
	}
}

func (b *Backend) Detach(_ context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	cancel := b.cancel
	waitCh := b.waitCh
	userDataDir := b.userDataDir
	b.cmd = nil
	b.cancel = nil
	b.waitCh = nil
	b.userDataDir = ""
	b.devtoolsURL = ""
	b.mu.Unlock()

	if cmd == nil {
		return nil
	}

	cancel()

	timer := time.NewTimer(shutdownTimeout)
	defer timer.Stop()

	select {
	case <-waitCh:
	case <-timer.C:
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGKILL)
		}
		<-waitCh
	}

	return os.RemoveAll(userDataDir)
}

func (b *Backend) Observe(ctx context.Context, opts api.ObserveOptions) (*api.Observation, error) {
	b.mu.Lock()
	url := b.devtoolsURL
	b.mu.Unlock()

	if url == "" {
		return nil, errors.New("chromium backend is not attached")
	}

	return b.observeViaCDP(ctx, url, opts)
}

func (b *Backend) Act(ctx context.Context, action api.Action) (*api.ActionResult, error) {
	b.mu.Lock()
	url := b.devtoolsURL
	b.mu.Unlock()

	if url == "" {
		return nil, errors.New("chromium backend is not attached")
	}

	switch action.Kind {
	case "back":
		return b.backViaCDP(ctx, url)
	case "dblclick":
		return b.mouseNodeViaCDP(ctx, url, action, "dblclick")
	case "get":
		return b.getViaCDP(ctx, url, action)
	case "hover":
		return b.mouseNodeViaCDP(ctx, url, action, "hover")
	case "invoke":
		return b.invokeViaCDP(ctx, url, action)
	case "key":
		return b.keyViaCDP(ctx, url, action)
	case "rightclick":
		return b.mouseNodeViaCDP(ctx, url, action, "rightclick")
	case "select":
		return b.selectViaCDP(ctx, url, action)
	case "wait":
		return b.waitViaCDP(ctx, url, action)
	case "scroll":
		return b.scrollViaCDP(ctx, url, action)
	case "type":
		return b.typeViaCDP(ctx, url, action)
	case "upload":
		return b.uploadViaCDP(ctx, url, action)
	case "eval":
		return b.evalViaCDP(ctx, url, action)
	case "viewport":
		return b.viewportViaCDP(ctx, url, action)
	default:
		return nil, fmt.Errorf("%w: %s", spec.ErrUnsupported, action.Kind)
	}
}

func (*Backend) Screenshot(context.Context, string) error {
	return nil
}

func (b *Backend) Logs(context.Context, api.LogOptions) ([]api.LogEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.logs) == 0 {
		return nil, nil
	}

	logs := append([]api.LogEntry(nil), b.logs...)
	return logs, nil
}

func (b *Backend) captureLogs(reader io.Reader, startedCh chan<- string) {
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 1024)

	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			for {
				index := strings.IndexByte(string(buf), '\n')
				if index < 0 {
					break
				}
				line := strings.TrimSpace(string(buf[:index]))
				buf = buf[index+1:]
				if line != "" {
					b.appendLog(line)
					if url, ok := strings.CutPrefix(line, "DevTools listening on "); ok {
						select {
						case startedCh <- url:
						default:
						}
					}
				}
			}
		}

		if err != nil {
			if len(buf) > 0 {
				line := strings.TrimSpace(string(buf))
				if line != "" {
					b.appendLog(line)
					if url, ok := strings.CutPrefix(line, "DevTools listening on "); ok {
						select {
						case startedCh <- url:
						default:
						}
					}
				}
			}
			return
		}
	}
}

func (b *Backend) appendLog(message string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs = append(b.logs, api.LogEntry{
		Time:    time.Now(),
		Level:   "info",
		Message: message,
	})
	if len(b.logs) > maxLogEntries {
		b.logs = append([]api.LogEntry(nil), b.logs[len(b.logs)-maxLogEntries:]...)
	}
}

func (b *Backend) cleanupAfterExit() {
	b.mu.Lock()
	userDataDir := b.userDataDir
	b.cmd = nil
	b.cancel = nil
	b.waitCh = nil
	b.userDataDir = ""
	b.devtoolsURL = ""
	b.mu.Unlock()

	if userDataDir != "" {
		os.RemoveAll(userDataDir)
	}
}

func sanitize(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	sanitized := replacer.Replace(value)
	if sanitized == "" {
		return "session"
	}
	return sanitized
}

func initialURL(options map[string]string) string {
	if options != nil && options["initial_url"] != "" {
		return options["initial_url"]
	}
	return "about:blank"
}

func viewportWidth(options map[string]string) int {
	return viewportOption(options, "viewport_width", defaultViewportWidth)
}

func viewportHeight(options map[string]string) int {
	return viewportOption(options, "viewport_height", defaultViewportHeight)
}

func viewportOption(options map[string]string, key string, fallback int) int {
	if options == nil {
		return fallback
	}

	value, err := strconv.Atoi(strings.TrimSpace(options[key]))
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}

type pageTargetInfo struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func ObserveViaCDP(ctx context.Context, devtoolsURL string, opts api.ObserveOptions, allocatorOptions ...chromedp.RemoteAllocatorOption) (*api.Observation, error) {
	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL, allocatorOptions...)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var currentURL string
	var title string
	var text string
	var treeJSON string
	var screenshot []byte
	actions := []chromedp.Action{
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
	}
	if opts.WithText {
		actions = append(actions, chromedp.Evaluate(`document.body ? document.body.innerText : ""`, &text))
	}
	if opts.WithTree {
		actions = append(actions, chromedp.Evaluate(observeTreeJS, &treeJSON))
	}
	if opts.WithScreenshot {
		if opts.FullScreenshot {
			actions = append(actions, chromedp.FullScreenshot(&screenshot, 100))
		} else {
			actions = append(actions, chromedp.CaptureScreenshot(&screenshot))
		}
	}

	if err := chromedp.Run(targetCtx, actions...); err != nil {
		return nil, err
	}

	tree, err := parseTreeJSON(treeJSON)
	if err != nil {
		return nil, err
	}

	return &api.Observation{
		URLOrScreen: currentURL,
		Title:       title,
		Text:        strings.TrimSpace(text),
		Tree:        tree,
		Screenshot:  base64.StdEncoding.EncodeToString(screenshot),
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func NavigateViaCDP(ctx context.Context, devtoolsURL string, navigateURL string, allocatorOptions ...chromedp.RemoteAllocatorOption) error {
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL, allocatorOptions...)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx)
	defer targetCancel()

	return chromedp.Run(targetCtx, chromedp.Navigate(navigateURL))
}

func (b *Backend) observeViaCDP(ctx context.Context, devtoolsURL string, opts api.ObserveOptions) (*api.Observation, error) {
	return ObserveViaCDP(ctx, devtoolsURL, opts)
}

func (b *Backend) evalViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("eval script is required")
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var value interface{}
	if err := chromedp.Run(targetCtx, chromedp.Evaluate(evalExpression(action.Text), &value, chromedp.EvalAsValue)); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: false,
		Value:   value,
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) viewportViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.Args == nil {
		return nil, errors.New("viewport width and height are required")
	}

	width, err := strconv.Atoi(strings.TrimSpace(action.Args["width"]))
	if err != nil || width <= 0 {
		return nil, errors.New("viewport width must be a positive integer")
	}
	height, err := strconv.Atoi(strings.TrimSpace(action.Args["height"]))
	if err != nil || height <= 0 {
		return nil, errors.New("viewport height must be a positive integer")
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	if err := chromedp.Run(targetCtx, chromedp.EmulateViewport(int64(width), int64(height))); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: fmt.Sprintf("set viewport %dx%d", width, height),
		Value: map[string]interface{}{
			"width":  width,
			"height": height,
		},
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) invokeViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var (
		message string
		value   map[string]interface{}
	)
	switch {
	case action.NodeID != nil:
		if *action.NodeID <= 0 {
			return nil, errors.New("invoke node_id must be positive")
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(clickExpression(*action.NodeID), &value, chromedp.EvalAsValue)); err != nil {
			return nil, err
		}
		message = fmt.Sprintf("clicked %d", *action.NodeID)
	case action.Args != nil:
		x, err := strconv.Atoi(strings.TrimSpace(action.Args["x"]))
		if err != nil || x < 0 {
			return nil, errors.New("invoke x coordinate must be a non-negative integer")
		}
		y, err := strconv.Atoi(strings.TrimSpace(action.Args["y"]))
		if err != nil || y < 0 {
			return nil, errors.New("invoke y coordinate must be a non-negative integer")
		}
		if err := chromedp.Run(targetCtx, chromedp.MouseClickXY(float64(x), float64(y))); err != nil {
			return nil, err
		}
		value = map[string]interface{}{
			"x": x,
			"y": y,
		}
		message = fmt.Sprintf("clicked %d %d", x, y)
	default:
		return nil, errors.New("invoke requires node_id or x y coordinates")
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: message,
		Value:   value,
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) mouseNodeViaCDP(ctx context.Context, devtoolsURL string, action api.Action, kind string) (*api.ActionResult, error) {
	if action.NodeID == nil || *action.NodeID <= 0 {
		return nil, fmt.Errorf("%s requires a positive index", kind)
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var point nodePoint
	if err := chromedp.Run(targetCtx, chromedp.Evaluate(nodePointExpression(*action.NodeID), &point, chromedp.EvalAsValue)); err != nil {
		return nil, err
	}

	var (
		actionErr error
		message   string
	)
	switch kind {
	case "hover":
		actionErr = chromedp.Run(targetCtx, chromedp.MouseEvent(input.MouseMoved, point.X, point.Y))
		message = fmt.Sprintf("hovered %d", *action.NodeID)
	case "dblclick":
		actionErr = chromedp.Run(targetCtx, chromedp.MouseClickXY(point.X, point.Y, chromedp.ClickCount(2)))
		message = fmt.Sprintf("double-clicked %d", *action.NodeID)
	case "rightclick":
		actionErr = chromedp.Run(targetCtx, chromedp.MouseClickXY(point.X, point.Y, chromedp.ButtonType(input.Right)))
		message = fmt.Sprintf("right-clicked %d", *action.NodeID)
	default:
		return nil, fmt.Errorf("unsupported mouse action: %s", kind)
	}
	if actionErr != nil {
		return nil, actionErr
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: message,
		Value: map[string]interface{}{
			"id":  point.ID,
			"tag": point.Tag,
			"x":   point.X,
			"y":   point.Y,
		},
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) typeViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("type text is required")
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var nodeID int
	if action.NodeID != nil {
		nodeID = *action.NodeID
		if nodeID <= 0 {
			return nil, errors.New("type node_id must be positive")
		}
	}

	message := "typed"
	if nodeID > 0 {
		message = fmt.Sprintf("typed into %d", nodeID)
	}

	token := fmt.Sprintf("nexus-type-%d", time.Now().UnixNano())
	var targetValue typeTarget
	if err := chromedp.Run(targetCtx, chromedp.Evaluate(markTypeTargetExpression(nodeID, token), &targetValue, chromedp.EvalAsValue)); err != nil {
		return nil, err
	}
	defer func() {
		_ = chromedp.Run(targetCtx, chromedp.Evaluate(clearMarkedTypeTargetExpression(token), nil))
	}()

	value := map[string]interface{}{
		"id":   targetValue.ID,
		"tag":  targetValue.Tag,
		"text": action.Text,
	}
	if err := chromedp.Run(targetCtx, chromedp.SendKeys(targetValue.Selector, action.Text, chromedp.ByQuery)); err == nil {
		value["method"] = "key_events"
	} else {
		var fallback map[string]interface{}
		if fallbackErr := chromedp.Run(targetCtx, chromedp.Evaluate(typeExpression(nodeID, action.Text), &fallback, chromedp.EvalAsValue)); fallbackErr != nil {
			return nil, err
		}
		for key, v := range fallback {
			value[key] = v
		}
		value["method"] = "dom_set"
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: message,
		Value:   value,
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) keyViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if len(action.Keys) != 1 || strings.TrimSpace(action.Keys[0]) == "" {
		return nil, errors.New("key requires a key spec")
	}

	keySpec := strings.TrimSpace(action.Keys[0])
	keyValue, modifiers, err := parseKeySpec(keySpec)
	if err != nil {
		return nil, err
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	if err := chromedp.Run(targetCtx, chromedp.KeyEvent(keyValue, chromedp.KeyModifiers(modifiers...))); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: fmt.Sprintf("sent keys %s", keySpec),
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) backViaCDP(ctx context.Context, devtoolsURL string) (*api.ActionResult, error) {
	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	if err := chromedp.Run(targetCtx, chromedp.Evaluate(`history.back()`, nil)); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: "went back",
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) getViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.Args == nil || strings.TrimSpace(action.Args["target"]) == "" {
		return nil, errors.New("get target is required")
	}

	targetKind := strings.TrimSpace(action.Args["target"])
	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var value interface{}
	switch targetKind {
	case "title":
		var title string
		if err := chromedp.Run(targetCtx, chromedp.Title(&title)); err != nil {
			return nil, err
		}
		value = title
	case "html":
		selector := strings.TrimSpace(action.Args["selector"])
		var html string
		if selector == "" {
			if err := chromedp.Run(targetCtx, chromedp.Evaluate(`document.documentElement ? document.documentElement.outerHTML : ""`, &html)); err != nil {
				return nil, err
			}
		} else {
			if err := chromedp.Run(targetCtx, chromedp.Evaluate(getHTMLExpression(selector), &html)); err != nil {
				return nil, err
			}
		}
		value = html
	case "text", "value", "attributes", "bbox":
		if action.NodeID == nil || *action.NodeID <= 0 {
			return nil, fmt.Errorf("get %s requires a positive index", targetKind)
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(getNodeExpression(targetKind, *action.NodeID), &value, chromedp.EvalAsValue)); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported get target: %s", targetKind)
	}

	return &api.ActionResult{
		OK:      true,
		Changed: false,
		Message: fmt.Sprintf("got %s", targetKind),
		Value:   value,
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) selectViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.NodeID == nil || *action.NodeID <= 0 {
		return nil, errors.New("select requires a positive index")
	}
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("select value is required")
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var value map[string]interface{}
	if err := chromedp.Run(targetCtx, chromedp.Evaluate(selectExpression(*action.NodeID, action.Text), &value, chromedp.EvalAsValue)); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: fmt.Sprintf("selected %s on %d", action.Text, *action.NodeID),
		Value:   value,
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) uploadViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.NodeID == nil || *action.NodeID <= 0 {
		return nil, errors.New("upload requires a positive index")
	}
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("upload path is required")
	}
	if _, err := os.Stat(action.Text); err != nil {
		return nil, err
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	token := fmt.Sprintf("nexus-upload-%d", time.Now().UnixNano())
	var marked map[string]interface{}
	if err := chromedp.Run(
		targetCtx,
		chromedp.Evaluate(markUploadNodeExpression(*action.NodeID, token), &marked, chromedp.EvalAsValue),
		chromedp.SetUploadFiles(`[data-nexus-upload="`+token+`"]`, []string{action.Text}, chromedp.ByQuery),
	); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: fmt.Sprintf("uploaded %s to %d", action.Text, *action.NodeID),
		Value:   marked,
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) scrollViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.Dir != "up" && action.Dir != "down" {
		return nil, errors.New("scroll dir must be up or down")
	}

	nodeID := 0
	if action.NodeID != nil {
		nodeID = *action.NodeID
		if nodeID <= 0 {
			return nil, errors.New("scroll node_id must be positive")
		}
	}

	amount := 0
	if action.Args != nil && action.Args["amount"] != "" {
		parsed, err := strconv.Atoi(action.Args["amount"])
		if err != nil || parsed < 0 {
			return nil, errors.New("scroll amount must be a non-negative integer")
		}
		amount = parsed
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var value map[string]interface{}
	if err := chromedp.Run(targetCtx, chromedp.Evaluate(scrollExpression(nodeID, action.Dir, amount), &value, chromedp.EvalAsValue)); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: true,
		Message: fmt.Sprintf("scrolled %s", action.Dir),
		Value:   value,
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func (b *Backend) waitViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.Args == nil {
		return nil, errors.New("wait target is required")
	}

	targetType := strings.TrimSpace(action.Args["target"])
	value := strings.TrimSpace(action.Args["value"])
	if targetType == "" || value == "" {
		return nil, errors.New("wait target is required")
	}

	timeout := 30 * time.Second
	if raw := strings.TrimSpace(action.Args["timeout_ms"]); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err != nil || ms < 0 {
			return nil, errors.New("wait timeout must be a non-negative integer")
		}
		timeout = time.Duration(ms) * time.Millisecond
	}

	targetInfo, err := currentPageTarget(ctx, devtoolsURL)
	if err != nil {
		return nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, devtoolsURL)
	defer allocCancel()

	targetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	defer targetCancel()

	var expression string
	switch targetType {
	case "selector":
		state := strings.TrimSpace(action.Args["state"])
		if state == "" {
			state = "visible"
		}
		expression, err = waitSelectorExpression(value, state)
		if err != nil {
			return nil, err
		}
	case "text":
		expression = waitTextExpression(value)
	case "url":
		expression = waitURLExpression(value)
	default:
		return nil, fmt.Errorf("unsupported wait target: %s", targetType)
	}

	if err := waitForExpression(targetCtx, expression, timeout); err != nil {
		return nil, err
	}

	return &api.ActionResult{
		OK:      true,
		Changed: false,
		Message: fmt.Sprintf("waited for %s", targetType),
		Meta: map[string]string{
			"devtools_url":   devtoolsURL,
			"page_target_id": targetInfo.ID,
		},
	}, nil
}

func waitForExpression(ctx context.Context, expression string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var ready bool
		err := chromedp.Run(ctx, chromedp.Evaluate(expression, &ready, chromedp.EvalAsValue))
		if err == nil && ready {
			return nil
		}
		if err != nil && !isRetryableWaitError(err) {
			return err
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("wait timed out after %s", timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func isRetryableWaitError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "Execution context was destroyed") ||
		strings.Contains(message, "Cannot find context with specified id")
}

func evalExpression(source string) string {
	return "(async () => await eval(" + strconv.Quote(source) + "))()"
}

func clickExpression(nodeID int) string {
	return strings.ReplaceAll(clickNodeJS, "$NODE_ID$", strconv.Itoa(nodeID))
}

func nodePointExpression(nodeID int) string {
	return strings.ReplaceAll(nodePointJS, "$NODE_ID$", strconv.Itoa(nodeID))
}

func markTypeTargetExpression(nodeID int, token string) string {
	script := strings.ReplaceAll(markTypeTargetJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$TOKEN$", strconv.Quote(token))
}

func clearMarkedTypeTargetExpression(token string) string {
	return strings.ReplaceAll(clearMarkedTypeTargetJS, "$TOKEN$", strconv.Quote(token))
}

func typeExpression(nodeID int, text string) string {
	script := strings.ReplaceAll(typeNodeJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$TEXT$", strconv.Quote(text))
}

func scrollExpression(nodeID int, dir string, amount int) string {
	script := strings.ReplaceAll(scrollJS, "$NODE_ID$", strconv.Itoa(nodeID))
	script = strings.ReplaceAll(script, "$DIR$", strconv.Quote(dir))
	return strings.ReplaceAll(script, "$AMOUNT$", strconv.Itoa(amount))
}

func getHTMLExpression(selector string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) {
    throw new Error('selector not found');
  }
  return el.outerHTML;
})()`
}

func getNodeExpression(kind string, nodeID int) string {
	script := strings.ReplaceAll(getNodeJS, "$KIND$", strconv.Quote(kind))
	return strings.ReplaceAll(script, "$NODE_ID$", strconv.Itoa(nodeID))
}

func selectExpression(nodeID int, value string) string {
	script := strings.ReplaceAll(selectNodeJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$VALUE$", strconv.Quote(value))
}

func markUploadNodeExpression(nodeID int, token string) string {
	script := strings.ReplaceAll(markUploadNodeJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$TOKEN$", strconv.Quote(token))
}

func waitTextExpression(value string) string {
	return `(document.body ? document.body.innerText : "").includes(` + strconv.Quote(value) + `)`
}

func waitURLExpression(value string) string {
	return `(window.location.href || "").includes(` + strconv.Quote(value) + `)`
}

func waitSelectorExpression(selector string, state string) (string, error) {
	switch state {
	case "attached":
		return `(document.querySelector(` + strconv.Quote(selector) + `) !== null)`, nil
	case "detached":
		return `(document.querySelector(` + strconv.Quote(selector) + `) === null)`, nil
	case "visible":
		return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) return false;
  const style = window.getComputedStyle(el);
  if (style.display === 'none' || style.visibility === 'hidden') return false;
  if (el.hidden) return false;
  const rect = el.getBoundingClientRect();
  return rect.width > 0 && rect.height > 0;
})()`, nil
	case "hidden":
		return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) return true;
  const style = window.getComputedStyle(el);
  if (style.display === 'none' || style.visibility === 'hidden') return true;
  if (el.hidden) return true;
  const rect = el.getBoundingClientRect();
  return rect.width === 0 || rect.height === 0;
})()`, nil
	default:
		return "", errors.New("wait selector state must be attached, detached, visible, or hidden")
	}
}

func parseKeySpec(spec string) (string, []input.Modifier, error) {
	parts := strings.Split(spec, "+")
	if len(parts) == 0 {
		return "", nil, errors.New("key requires a key spec")
	}

	var modifiers []input.Modifier
	for _, part := range parts[:len(parts)-1] {
		modifier, ok := lookupModifier(part)
		if !ok {
			return "", nil, fmt.Errorf("unknown key modifier: %s", part)
		}
		modifiers = append(modifiers, modifier)
	}

	keyPart := strings.TrimSpace(parts[len(parts)-1])
	if keyPart == "" {
		return "", nil, errors.New("key requires a key value")
	}

	if keyValue, ok := lookupSpecialKey(keyPart); ok {
		return keyValue, modifiers, nil
	}

	if len([]rune(keyPart)) == 1 {
		if containsModifier(modifiers, input.ModifierShift) {
			return keyPart, modifiers, nil
		}
		return strings.ToLower(keyPart), modifiers, nil
	}

	return keyPart, modifiers, nil
}

func lookupModifier(value string) (input.Modifier, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "alt", "option":
		return input.ModifierAlt, true
	case "ctrl", "control":
		return input.ModifierCtrl, true
	case "cmd", "command", "meta", "super":
		return input.ModifierMeta, true
	case "shift":
		return input.ModifierShift, true
	default:
		return 0, false
	}
}

func lookupSpecialKey(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "backspace":
		return kb.Backspace, true
	case "tab":
		return kb.Tab, true
	case "enter", "return":
		return kb.Enter, true
	case "escape", "esc":
		return kb.Escape, true
	case "delete", "del":
		return kb.Delete, true
	case "space":
		return " ", true
	case "arrowdown", "down":
		return kb.ArrowDown, true
	case "arrowleft", "left":
		return kb.ArrowLeft, true
	case "arrowright", "right":
		return kb.ArrowRight, true
	case "arrowup", "up":
		return kb.ArrowUp, true
	case "end":
		return kb.End, true
	case "home":
		return kb.Home, true
	case "pagedown":
		return kb.PageDown, true
	case "pageup":
		return kb.PageUp, true
	default:
		return "", false
	}
}

func containsModifier(modifiers []input.Modifier, target input.Modifier) bool {
	for _, modifier := range modifiers {
		if modifier == target {
			return true
		}
	}
	return false
}

func currentPageTarget(ctx context.Context, devtoolsURL string) (pageTargetInfo, error) {
	deadline := time.Now().Add(pageTargetTimeout)
	for {
		target, err := currentPageTargetOnce(ctx, devtoolsURL)
		if err == nil {
			return target, nil
		}
		if !errors.Is(err, errPageTargetNotFound) && !isRetryablePageTargetError(err) {
			return pageTargetInfo{}, err
		}
		if time.Now().After(deadline) {
			return pageTargetInfo{}, err
		}

		select {
		case <-ctx.Done():
			return pageTargetInfo{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func currentPageTargetOnce(ctx context.Context, devtoolsURL string) (pageTargetInfo, error) {
	baseURL, err := debugHTTPBaseURL(devtoolsURL)
	if err != nil {
		return pageTargetInfo{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/json/list", nil)
	if err != nil {
		return pageTargetInfo{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return pageTargetInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return pageTargetInfo{}, errors.New("failed to list page targets")
	}

	var targets []pageTargetInfo
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return pageTargetInfo{}, err
	}

	for _, target := range targets {
		if target.Type == "page" {
			return target, nil
		}
	}

	return pageTargetInfo{}, errPageTargetNotFound
}

func isRetryablePageTargetError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "connection refused") ||
		strings.Contains(message, "EOF") ||
		strings.Contains(message, "reset by peer")
}

func debugHTTPBaseURL(devtoolsURL string) (string, error) {
	parsed, err := url.Parse(devtoolsURL)
	if err != nil {
		return "", err
	}

	scheme := "http"
	switch parsed.Scheme {
	case "ws":
		scheme = "http"
	case "wss":
		scheme = "https"
	case "http", "https":
		scheme = parsed.Scheme
	default:
		return "", errors.New("unsupported devtools url scheme")
	}

	if parsed.Host == "" {
		return "", errors.New("devtools url host is empty")
	}

	return scheme + "://" + parsed.Host, nil
}

type rawNode struct {
	ID          int               `json:"id"`
	Fingerprint string            `json:"fingerprint"`
	Role        string            `json:"role"`
	Name        string            `json:"name"`
	Text        string            `json:"text"`
	Value       string            `json:"value"`
	Bounds      api.Rect          `json:"bounds"`
	Visible     bool              `json:"visible"`
	Enabled     bool              `json:"enabled"`
	Focused     bool              `json:"focused"`
	Editable    bool              `json:"editable"`
	Selectable  bool              `json:"selectable"`
	Invokable   bool              `json:"invokable"`
	Scrollable  bool              `json:"scrollable"`
	Children    []int             `json:"children"`
	Attrs       map[string]string `json:"attrs"`
	ParentID    *int              `json:"parent_id"`
}

func parseTreeJSON(treeJSON string) ([]api.Node, error) {
	if strings.TrimSpace(treeJSON) == "" {
		return nil, nil
	}

	var raw []rawNode
	if err := json.Unmarshal([]byte(treeJSON), &raw); err != nil {
		return nil, err
	}

	nodes := make([]api.Node, 0, len(raw))
	for _, node := range raw {
		nodes = append(nodes, api.Node{
			ID:          node.ID,
			Fingerprint: strings.TrimSpace(node.Fingerprint),
			Role:        node.Role,
			Name:        strings.TrimSpace(node.Name),
			Text:        strings.TrimSpace(node.Text),
			Value:       strings.TrimSpace(node.Value),
			Bounds:      node.Bounds,
			Visible:     node.Visible,
			Enabled:     node.Enabled,
			Focused:     node.Focused,
			Editable:    node.Editable,
			Selectable:  node.Selectable,
			Invokable:   node.Invokable,
			Scrollable:  node.Scrollable,
			Children:    node.Children,
			Attrs:       node.Attrs,
		})
	}

	return nodes, nil
}
