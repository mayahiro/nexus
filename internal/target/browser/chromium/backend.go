package chromium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/mayahiro/nexus/internal/api"
	"github.com/mayahiro/nexus/internal/target/browser/spec"
)

const startupTimeout = 5 * time.Second
const shutdownTimeout = 5 * time.Second
const maxLogEntries = 200
const defaultViewportWidth = 1920
const defaultViewportHeight = 1080
const screenshotAttemptTimeout = 10 * time.Second
const maxFullScreenshotWidth = 16384
const maxFullScreenshotHeight = 50000
const maxFullScreenshotPixels = 120_000_000

var pageTargetTimeout = 5 * time.Second

func selectorHintSupportExpression() string {
	return `
  const selectorHintNormalize = (value) => (value || '').trim().replace(/\s+/g, ' ').slice(0, 80);

  const selectorHintEscape = (value) => {
    if (window.CSS && typeof window.CSS.escape === 'function') return window.CSS.escape(value);
    return value.replace(/[^a-zA-Z0-9_-]/g, '\\$&');
  };

  const selectorHintQuote = (value) => '"' + selectorHintNormalize(value).replace(/"/g, '\\"') + '"';

  const selectorHintSelector = (el) => {
    const tag = el.tagName ? el.tagName.toLowerCase() : 'element';
    if (el.id) return tag + '#' + selectorHintEscape(el.id);
    const testIDName = el.getAttribute('data-testid') ? 'data-testid' : 'data-test';
    const testID = el.getAttribute(testIDName);
    if (testID) return tag + '[' + testIDName + '="' + testID.replace(/"/g, '\\"') + '"]';
    const classes = Array.from(el.classList || []).filter(Boolean).slice(0, 3);
    if (classes.length > 0) return tag + classes.map((value) => '.' + selectorHintEscape(value)).join('');
    return tag;
  };

  const selectorHintRole = (el) => {
    const ariaRole = (el.getAttribute('role') || '').trim();
    if (ariaRole) return ariaRole;
    const tag = el.tagName ? el.tagName.toLowerCase() : '';
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

  const selectorHintName = (el) => {
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
    if (el.labels && el.labels.length) {
      const text = Array.from(el.labels)
        .map((label) => (label.innerText || label.textContent || '').trim())
        .join(' ')
        .trim();
      if (text) return text;
    }
    return '';
  };

  const selectorHintFor = (el, index) => {
    const parts = ['#' + (index + 1), selectorHintSelector(el)];
    const role = selectorHintRole(el);
    const name = selectorHintName(el);
    const testID = (el.getAttribute('data-testid') || el.getAttribute('data-test') || '').trim();
    const text = selectorHintNormalize(el.innerText || el.textContent || '');
    const rect = el.getBoundingClientRect();
    if (role) parts.push('role=' + selectorHintQuote(role));
    if (name) parts.push('name=' + selectorHintQuote(name));
    if (testID) parts.push('testid=' + selectorHintQuote(testID));
    if (text && text !== selectorHintNormalize(name)) parts.push('text=' + selectorHintQuote(text));
    parts.push('bbox=' + Math.round(rect.x) + ',' + Math.round(rect.y) + ' ' + Math.round(rect.width) + 'x' + Math.round(rect.height));
    return parts.join(' ');
  };

  const selectorHintSuffix = (matches) => {
    const hints = matches.slice(0, 5).map(selectorHintFor).join('; ');
    if (!hints) return '';
    return '. candidates: ' + hints + (matches.length > 5 ? '; ...' : '');
  };
`
}

func observeTreeExpression(cssProperties []string, scopeSelector string, layoutProperties []string, nodeScope string) string {
	return observeTreeExpressionWithSelector(cssProperties, scopeSelector, layoutProperties, nodeScope, "", false)
}

func observeTreeExpressionWithSelector(cssProperties []string, scopeSelector string, layoutProperties []string, nodeScope string, matchSelector string, excludeScopeRoot bool) string {
	properties := make([]string, 0, len(cssProperties))
	for _, value := range cssProperties {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		properties = append(properties, strconv.Quote(trimmed))
	}
	layout := make([]string, 0, len(layoutProperties))
	for _, value := range layoutProperties {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		layout = append(layout, strconv.Quote(trimmed))
	}

	scope := strconv.Quote(strings.TrimSpace(scopeSelector))
	includeScopeRoot := !excludeScopeRoot
	selectorValue := strings.TrimSpace(matchSelector)
	if selectorValue == "" {
		selectorValue = observeCandidateSelector(nodeScope)
	}
	candidateSelector := strconv.Quote(selectorValue)
	selectorHints := ""
	if strings.TrimSpace(scopeSelector) != "" || strings.TrimSpace(matchSelector) != "" {
		selectorHints = selectorHintSupportExpression()
	}

	return `(function () {
	  const scopeSelector = ` + scope + `;
	  const selector = ` + candidateSelector + `;
	  const includeScopeRoot = ` + strconv.FormatBool(includeScopeRoot) + `;

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
    if (el.labels && el.labels.length) {
      const text = Array.from(el.labels)
        .map((label) => (label.innerText || label.textContent || '').trim())
        .join(' ')
        .trim();
      if (text) return text;
    }
    const alt = (el.getAttribute('alt') || '').trim();
    if (alt) return alt;
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
    if (el.getAttribute('alt')) attrs.alt = el.getAttribute('alt');
    if (el.getAttribute('aria-label')) attrs['aria-label'] = el.getAttribute('aria-label');
    if (el.getAttribute('aria-labelledby')) attrs['aria-labelledby'] = el.getAttribute('aria-labelledby');
    if (el.getAttribute('aria-hidden')) attrs['aria-hidden'] = el.getAttribute('aria-hidden');
    if (el.hasAttribute('hidden')) attrs.hidden = el.getAttribute('hidden') || 'true';
    if (el.getAttribute('data-testid')) attrs['data-testid'] = el.getAttribute('data-testid');
    if (el.getAttribute('data-test')) attrs['data-test'] = el.getAttribute('data-test');
    if (el.getAttribute('data-nxctl-skip')) attrs['data-nxctl-skip'] = el.getAttribute('data-nxctl-skip');
    return attrs;
  };

  const normalize = (value) => (value || '').trim().replace(/\s+/g, ' ').slice(0, 80);
  const normalizeFullText = (value) => (value || '').trim().replace(/\s+/g, ' ');

  const sameTagOrdinal = (el) => {
    if (!el.parentElement) return 1;
    const tag = el.tagName.toLowerCase();
    let ordinal = 0;
    for (const child of el.parentElement.children) {
      if (child.tagName.toLowerCase() !== tag) continue;
      ordinal++;
      if (child === el) return ordinal;
    }
    return ordinal || 1;
  };

  const structurePathFor = (el) => {
    const parts = [];
    for (let current = el; current; current = current.parentElement) {
      const tag = current.tagName.toLowerCase();
      parts.push(tag + ':' + sameTagOrdinal(current));
      if (tag === 'html') break;
    }
    return parts.reverse().join('>');
  };

  const cssProperties = [` + strings.Join(properties, ",") + `];
  const layoutProperties = [` + strings.Join(layout, ",") + `];
  const colorPropertyPattern = /(^|-)color$/;
  const colorPropertyNames = new Set(['fill', 'stroke']);
  const colorProbe = document.createElement('span');
  const colorCanvas = document.createElement('canvas');
  colorCanvas.width = 1;
  colorCanvas.height = 1;
  const colorContext = colorCanvas.getContext('2d', { colorSpace: 'srgb', willReadFrequently: true });

  const isColorProperty = (property) => colorPropertyPattern.test(property) || colorPropertyNames.has(property);

  const formatColorNumber = (value) => {
    const rounded = Math.round(value * 10000) / 10000;
    if (Math.abs(rounded) < 0.00005) return '0';
    return rounded.toFixed(4).replace(/\.?0+$/, '');
  };

  const normalizeColorValue = (value) => {
    if (!colorContext || !value) return value;

    colorProbe.style.color = '';
    colorProbe.style.color = value;
    if (!colorProbe.style.color) return value;

    colorContext.clearRect(0, 0, 1, 1);
    colorContext.globalCompositeOperation = 'copy';
    colorContext.fillStyle = value;
    colorContext.fillRect(0, 0, 1, 1);

    try {
      const imageData = colorContext.getImageData(0, 0, 1, 1, { colorSpace: 'srgb', pixelFormat: 'rgba-float16' });
      if (imageData && imageData.pixelFormat === 'rgba-float16' && imageData.data.length >= 4) {
        const red = Math.min(Math.max(imageData.data[0] * 255, 0), 255);
        const green = Math.min(Math.max(imageData.data[1] * 255, 0), 255);
        const blue = Math.min(Math.max(imageData.data[2] * 255, 0), 255);
        const alpha = Math.min(Math.max(imageData.data[3], 0), 1);
        if (alpha >= 0.99995) {
          return 'rgb(' + formatColorNumber(red) + ', ' + formatColorNumber(green) + ', ' + formatColorNumber(blue) + ')';
        }
        return 'rgba(' + formatColorNumber(red) + ', ' + formatColorNumber(green) + ', ' + formatColorNumber(blue) + ', ' + formatColorNumber(alpha) + ')';
      }
    } catch (error) {
    }

    const imageData = colorContext.getImageData(0, 0, 1, 1);
    if (!imageData || imageData.data.length < 4) return value;

    const alpha = imageData.data[3] / 255;
    if (imageData.data[3] === 255) {
      return 'rgb(' + imageData.data[0] + ', ' + imageData.data[1] + ', ' + imageData.data[2] + ')';
    }
    return 'rgba(' + imageData.data[0] + ', ' + imageData.data[1] + ', ' + imageData.data[2] + ', ' + formatColorNumber(alpha) + ')';
  };

  const normalizeStyleValue = (property, value) => {
    if (!isColorProperty(property)) return value;
    return normalizeColorValue(value);
  };

  const stylesFor = (el, properties) => {
    if (properties.length === 0) return {};
    const style = window.getComputedStyle(el);
    const values = {};
    const selectedProperties = properties.includes('*') ? Array.from(style) : properties;
    for (const property of selectedProperties) {
      values[property] = normalizeStyleValue(property, style.getPropertyValue(property).trim());
    }
    return values;
  };

  const escapeSelectorPart = (value) => {
    if (window.CSS && typeof window.CSS.escape === 'function') return window.CSS.escape(value);
    return value.replace(/[^a-zA-Z0-9_-]/g, '\\$&');
  };

  const selectorFor = (el) => {
    const tag = el.tagName.toLowerCase();
    if (el.id) return tag + '#' + escapeSelectorPart(el.id);
    const testIDName = el.getAttribute('data-testid') ? 'data-testid' : 'data-test';
    const testID = el.getAttribute(testIDName);
    if (testID) return tag + '[' + testIDName + '="' + testID.replace(/"/g, '\\"') + '"]';
    const classes = Array.from(el.classList || []).filter(Boolean).slice(0, 3);
    if (classes.length > 0) return tag + classes.map((value) => '.' + escapeSelectorPart(value)).join('');
    return tag;
  };

  const layoutAttrsFor = (el) => {
    const attrs = attrsFor(el);
    if (el.className && typeof el.className === 'string') attrs.class = normalize(el.className);
    return attrs;
  };

  const layoutContextFor = (el) => {
    if (layoutProperties.length === 0) return [];
    const context = [];
    for (let ancestor = el.parentElement; ancestor; ancestor = ancestor.parentElement) {
      const rect = ancestor.getBoundingClientRect();
      context.push({
        selector: selectorFor(ancestor),
        role: roleFor(ancestor),
        name: normalize(nameFor(ancestor)),
        styles: stylesFor(ancestor, layoutProperties),
        bounds: {
          x: Math.round(rect.x),
          y: Math.round(rect.y),
          w: Math.round(rect.width),
          h: Math.round(rect.height)
        },
        scrollable: ancestor.scrollHeight > ancestor.clientHeight || ancestor.scrollWidth > ancestor.clientWidth,
        attrs: layoutAttrsFor(ancestor)
      });
    }
    return context;
  };

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
` + selectorHints + `

	  let scopeRoot = null;
	  if (scopeSelector) {
	    let scopeMatches;
	    try {
	      scopeMatches = Array.from(document.querySelectorAll(scopeSelector));
	    } catch (error) {
	      throw new Error('scope selector is invalid: ' + scopeSelector);
	    }
	    if (scopeMatches.length === 0) {
	      throw new Error('scope selector matched 0 nodes: ' + scopeSelector);
	    }
	    if (scopeMatches.length !== 1) {
	      throw new Error('scope selector matched ' + scopeMatches.length + ' nodes: ' + scopeSelector + selectorHintSuffix(scopeMatches));
	    }
	    scopeRoot = scopeMatches[0];
	  }

	  let selectorMatches;
	  try {
	    selectorMatches = Array.from((scopeRoot || document).querySelectorAll(selector));
	  } catch (error) {
	    throw new Error('match selector is invalid: ' + selector);
	  }

	  const baseCandidates = selectorMatches
	    .filter((el) => visible(el));

	  const candidates = [];
	  if (scopeRoot && includeScopeRoot) {
	    candidates.push(scopeRoot);
	  }
	  for (const el of baseCandidates) {
	    if (scopeRoot && el === scopeRoot) continue;
	    candidates.push(el);
	  }

  const ids = new Map();
  candidates.forEach((el, index) => ids.set(el, index + 1));

  const nodes = candidates.map((el, index) => {
    const rect = el.getBoundingClientRect();
    const documentX = rect.x + (window.scrollX || window.pageXOffset || 0);
    const documentY = rect.y + (window.scrollY || window.pageYOffset || 0);
    const parentCandidate = candidates.find((candidate) => candidate !== el && candidate.contains(el) && !Array.from(candidates).some((other) => other !== candidate && other !== el && other.contains(el) && candidate.contains(other)));
    const children = candidates.filter((candidate) => candidate !== el && el.contains(candidate) && !Array.from(candidates).some((other) => other !== candidate && other !== el && el.contains(other) && other.contains(candidate))).map((child) => ids.get(child));

    const role = roleFor(el);
    const name = nameFor(el);
    const attrs = attrsFor(el);
    const styles = stylesFor(el, cssProperties);

    return {
      id: index + 1,
      fingerprint: fingerprintFor(el, role, name, attrs),
      structure_path: structurePathFor(el),
      text_length: normalizeFullText(el.innerText || el.textContent || '').length,
      descendants: el.querySelectorAll('*').length,
      role: role,
      name: name,
      text: textFor(el),
      value: valueFor(el),
      styles: styles,
      layout_context: layoutContextFor(el),
      bounds: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        w: Math.round(rect.width),
        h: Math.round(rect.height)
      },
      document_bounds: {
        x: Math.round(documentX),
        y: Math.round(documentY),
        w: Math.round(rect.width),
        h: Math.round(rect.height)
      },
      visible: visible(el),
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
}

func observeCandidateSelector(nodeScope string) string {
	current := []string{
		"button",
		"a[href]",
		"input",
		"textarea",
		"select",
		`[role="button"]`,
		`[role="link"]`,
		`[role="tab"]`,
		`[role="checkbox"]`,
		`[role="radio"]`,
		`[contenteditable="true"]`,
		`[contenteditable=""]`,
		"[onclick]",
		"[tabindex]",
	}
	switch strings.ToLower(strings.TrimSpace(nodeScope)) {
	case "actionable":
		return strings.Join(append(current,
			`[role="switch"]`,
			`[role="menuitem"]`,
			`[role="menuitemcheckbox"]`,
			`[role="menuitemradio"]`,
			`[role="option"]`,
			`[role="slider"]`,
			`[role="spinbutton"]`,
			`[role="searchbox"]`,
		), ",")
	case "semantic":
		return strings.Join(append(current,
			"h1",
			"h2",
			"h3",
			"h4",
			"h5",
			"h6",
			"main",
			"nav",
			"header",
			"footer",
			"section[aria-label]",
			"section[aria-labelledby]",
			"article",
			"table",
			"img[alt]",
			"[data-testid]",
			"[data-test]",
			`[role="alert"]`,
			`[role="article"]`,
			`[role="banner"]`,
			`[role="complementary"]`,
			`[role="contentinfo"]`,
			`[role="dialog"]`,
			`[role="figure"]`,
			`[role="form"]`,
			`[role="heading"]`,
			`[role="img"]`,
			`[role="main"]`,
			`[role="navigation"]`,
			`[role="region"]`,
			`[role="search"]`,
			`[role="status"]`,
			`[role="table"]`,
			`[role="toolbar"]`,
		), ",")
	case "all":
		return "*"
	default:
		return strings.Join(current, ",")
	}
}

func scopeTextExpression(scopeSelector string) string {
	return `(function () {
  const selector = ` + strconv.Quote(strings.TrimSpace(scopeSelector)) + `;
` + selectorHintSupportExpression() + `
  let matches;
  try {
    matches = Array.from(document.querySelectorAll(selector));
  } catch (error) {
    throw new Error('scope selector is invalid: ' + selector);
  }
  if (matches.length === 0) {
    throw new Error('scope selector matched 0 nodes: ' + selector);
  }
  if (matches.length !== 1) {
    throw new Error('scope selector matched ' + matches.length + ' nodes: ' + selector + selectorHintSuffix(matches));
  }
  const root = matches[0];
  return (root.innerText || root.textContent || '').trim();
})()`
}

func scopeMetaExpression(scopeSelector string) string {
	return `(function () {
  const selector = ` + strconv.Quote(strings.TrimSpace(scopeSelector)) + `;
` + selectorHintSupportExpression() + `
  let matches;
  try {
    matches = Array.from(document.querySelectorAll(selector));
  } catch (error) {
    throw new Error('scope selector is invalid: ' + selector);
  }
  if (matches.length === 0) {
    throw new Error('scope selector matched 0 nodes: ' + selector);
  }
  if (matches.length !== 1) {
    throw new Error('scope selector matched ' + matches.length + ' nodes: ' + selector + selectorHintSuffix(matches));
  }
  const root = matches[0];
  return {
    scope_selector: selector,
    scope_tag: root.tagName ? root.tagName.toLowerCase() : ''
  };
})()`
}

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
    const prototype = tag === 'input' ? window.HTMLInputElement.prototype : window.HTMLTextAreaElement.prototype;
    const valueDescriptor = Object.getOwnPropertyDescriptor(prototype, 'value');
    if (!valueDescriptor || typeof valueDescriptor.set !== 'function') {
      throw new Error('native value setter is unavailable');
    }
    valueDescriptor.set.call(el, text);
    if (typeof el.setSelectionRange === 'function') {
      try {
        el.setSelectionRange(text.length, text.length);
      } catch (_) {
      }
    }
  } else {
    el.textContent = text;
  }

  let inputEvent;
  try {
    inputEvent = new InputEvent('input', {
      bubbles: true,
      composed: true,
      data: text,
      inputType: 'insertReplacementText'
    });
  } catch (_) {
    inputEvent = new Event('input', {bubbles: true, composed: true});
  }
  el.dispatchEvent(inputEvent);
  el.dispatchEvent(new Event('change', {bubbles: true, composed: true}));

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
    try {
      el.setSelectionRange(el.value.length, el.value.length);
    } catch (_) {
    }
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

const installKeyProbeJS = `(function (token) {
  const probes = globalThis.__nexusKeyProbes || (globalThis.__nexusKeyProbes = {});
  const existing = probes[token];
  if (existing) {
    window.removeEventListener('keydown', existing.handler, true);
  }
  const state = {count: 0};
  state.handler = () => {
    state.count++;
  };
  probes[token] = state;
  window.addEventListener('keydown', state.handler, true);
  return true;
})($TOKEN$)`

const finishKeyProbeJS = `(function (token) {
  const probes = globalThis.__nexusKeyProbes || {};
  const state = probes[token];
  if (!state) return -1;
  window.removeEventListener('keydown', state.handler, true);
  delete probes[token];
  return state.count;
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
	mu                  sync.Mutex
	opMu                sync.Mutex
	cmd                 *exec.Cmd
	runCtx              context.Context
	cancel              context.CancelFunc
	waitCh              chan error
	userDataDir         string
	devtoolsURL         string
	logs                []api.LogEntry
	allocCtx            context.Context
	allocCancel         context.CancelFunc
	targetCtx           context.Context
	targetCancel        context.CancelFunc
	targetInfo          pageTargetInfo
	staleContexts       []remoteContext
	reattachAttempted   bool
	allocatorOptions    []chromedp.RemoteAllocatorOption
	refLoaderID         string
	refURL              string
	refs                map[int]nodeReference
	persistentContextID runtime.ExecutionContextID
	persistentLoaderID  string
	persistentWorldName string
	dialogOpen          bool
	dialogType          string
	dialogMessage       string
	activateBeforeOp    bool
}

var errPageTargetNotFound = errors.New("page target not found")
var errFullScreenshotTooLarge = errors.New("full screenshot exceeds capture limits")

type nodeReference struct {
	Selector string
	Identity string
}

type remoteContext struct {
	targetCancel context.CancelFunc
	allocCancel  context.CancelFunc
}

func New() *Backend {
	return &Backend{activateBeforeOp: true}
}

func (*Backend) Name() spec.BackendName {
	return spec.BackendChromium
}

func (*Backend) Capabilities() spec.Capabilities {
	return spec.Capabilities{
		Observe:       true,
		Act:           true,
		Screenshot:    true,
		Logs:          true,
		LayoutContext: true,
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return killProcessGroup(cmd.Process)
	}
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
	b.runCtx = runCtx
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
	b.opMu.Lock()
	defer b.opMu.Unlock()

	b.mu.Lock()
	cmd := b.cmd
	cancel := b.cancel
	allocCancel := b.allocCancel
	targetCancel := b.targetCancel
	staleContexts := append([]remoteContext(nil), b.staleContexts...)
	waitCh := b.waitCh
	userDataDir := b.userDataDir
	b.cmd = nil
	b.runCtx = nil
	b.cancel = nil
	b.waitCh = nil
	b.userDataDir = ""
	b.devtoolsURL = ""
	b.allocCtx = nil
	b.allocCancel = nil
	b.targetCtx = nil
	b.targetCancel = nil
	b.targetInfo = pageTargetInfo{}
	b.staleContexts = nil
	b.reattachAttempted = false
	b.refLoaderID = ""
	b.refURL = ""
	b.refs = nil
	b.persistentContextID = 0
	b.persistentLoaderID = ""
	b.persistentWorldName = ""
	b.dialogOpen = false
	b.dialogType = ""
	b.dialogMessage = ""
	b.mu.Unlock()

	if cmd == nil {
		return nil
	}

	cancel()
	if targetCancel != nil {
		targetCancel()
	}
	if allocCancel != nil {
		allocCancel()
	}
	cancelRemoteContexts(staleContexts)

	timer := time.NewTimer(shutdownTimeout)
	defer timer.Stop()

	select {
	case <-waitCh:
	case <-timer.C:
		killProcessGroup(cmd.Process)
		<-waitCh
	}

	return os.RemoveAll(userDataDir)
}

func killProcessGroup(process *os.Process) error {
	if process == nil {
		return nil
	}
	err := syscall.Kill(-process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

func (b *Backend) Observe(ctx context.Context, opts api.ObserveOptions) (*api.Observation, error) {
	b.opMu.Lock()
	defer b.opMu.Unlock()

	b.mu.Lock()
	url := b.devtoolsURL
	b.mu.Unlock()

	if url == "" {
		return nil, errors.New("chromium backend is not attached")
	}

	return b.observeViaCDP(ctx, url, opts)
}

func (b *Backend) Act(ctx context.Context, action api.Action) (*api.ActionResult, error) {
	b.opMu.Lock()
	defer b.opMu.Unlock()

	b.mu.Lock()
	url := b.devtoolsURL
	b.mu.Unlock()

	if url == "" {
		return nil, errors.New("chromium backend is not attached")
	}

	if strings.TrimSpace(action.NodeRef) != "" {
		selector, err := b.resolveNodeReference(ctx, url, action.NodeRef)
		if err != nil {
			return nil, err
		}
		action.Selector = selector
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
	case "navigate":
		return b.navigateViaCDP(ctx, url, action)
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
	case "fill":
		return b.fillViaCDP(ctx, url, action)
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
	allocCancel := b.allocCancel
	targetCancel := b.targetCancel
	staleContexts := append([]remoteContext(nil), b.staleContexts...)
	b.cmd = nil
	b.runCtx = nil
	b.cancel = nil
	b.waitCh = nil
	b.userDataDir = ""
	b.devtoolsURL = ""
	b.allocCtx = nil
	b.allocCancel = nil
	b.targetCtx = nil
	b.targetCancel = nil
	b.targetInfo = pageTargetInfo{}
	b.staleContexts = nil
	b.reattachAttempted = false
	b.refLoaderID = ""
	b.refURL = ""
	b.refs = nil
	b.persistentContextID = 0
	b.persistentLoaderID = ""
	b.persistentWorldName = ""
	b.dialogOpen = false
	b.dialogType = ""
	b.dialogMessage = ""
	b.mu.Unlock()

	if targetCancel != nil {
		targetCancel()
	}
	if allocCancel != nil {
		allocCancel()
	}
	cancelRemoteContexts(staleContexts)
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

func withBackendPageTargetContext[T any](b *Backend, ctx context.Context, devtoolsURL string, fn func(context.Context, pageTargetInfo) (T, error)) (T, error) {
	var zero T

	targetCtx, targetInfo, release, err := b.pageTargetContext(ctx, devtoolsURL)
	if err != nil {
		return zero, err
	}
	defer release()

	result, err := fn(targetCtx, targetInfo)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func (b *Backend) pageTargetContext(ctx context.Context, devtoolsURL string) (context.Context, pageTargetInfo, func(), error) {
	return b.pageTargetContextWithDependencies(ctx, devtoolsURL, currentPageTarget, initializePageTargetContext)
}

func (b *Backend) pageTargetContextWithDependencies(
	ctx context.Context,
	devtoolsURL string,
	resolveTarget func(context.Context, string) (pageTargetInfo, error),
	initializeTarget func(context.Context, context.Context) error,
) (context.Context, pageTargetInfo, func(), error) {
	b.mu.Lock()
	targetCtx := b.targetCtx
	targetInfo := b.targetInfo
	runCtx := b.runCtx
	b.mu.Unlock()

	if targetCtx == nil {
		if runCtx == nil {
			return nil, pageTargetInfo{}, nil, errors.New("chromium backend is not attached")
		}
		var err error
		targetInfo, err = resolveTarget(ctx, devtoolsURL)
		if err != nil {
			return nil, pageTargetInfo{}, nil, err
		}
		if err := b.activatePageTargetBeforeAttach(ctx, devtoolsURL, targetInfo.ID); err != nil {
			return nil, pageTargetInfo{}, nil, err
		}

		allocCtx, allocCancel := chromedp.NewRemoteAllocator(runCtx, devtoolsURL, b.allocatorOptions...)
		var targetCancel context.CancelFunc
		targetCtx, targetCancel = chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
		chromedp.ListenTarget(targetCtx, func(event any) {
			b.trackDialogEvent(targetInfo.ID, event)
		})
		if err := initializeTarget(ctx, targetCtx); err != nil {
			targetCancel()
			allocCancel()
			return nil, pageTargetInfo{}, nil, err
		}

		b.mu.Lock()
		b.allocCtx = allocCtx
		b.allocCancel = allocCancel
		b.targetCtx = targetCtx
		b.targetCancel = targetCancel
		b.targetInfo = targetInfo
		b.reattachAttempted = false
		b.mu.Unlock()
	}

	if targetCtx == nil {
		return nil, pageTargetInfo{}, nil, errors.New("chromium target context is unavailable")
	}
	operationCtx, release := operationContext(targetCtx, ctx)
	if err := b.activatePageTarget(operationCtx, targetInfo.ID); err != nil {
		release()
		return nil, pageTargetInfo{}, nil, err
	}

	return operationCtx, targetInfo, release, nil
}

func initializePageTargetContext(requestCtx context.Context, targetCtx context.Context) error {
	result := make(chan error, 1)
	go func() {
		result <- chromedp.Run(targetCtx)
	}()

	select {
	case err := <-result:
		return err
	case <-requestCtx.Done():
		return requestCtx.Err()
	}
}

func activateTarget(ctx context.Context, targetID string) error {
	return chromedp.Run(ctx, chromedp.ActionFunc(func(runCtx context.Context) error {
		chromedpContext := chromedp.FromContext(runCtx)
		if chromedpContext == nil || chromedpContext.Browser == nil {
			return errors.New("chromium browser context is unavailable")
		}
		return target.ActivateTarget(target.ID(targetID)).Do(cdp.WithExecutor(runCtx, chromedpContext.Browser))
	}))
}

func (b *Backend) activatePageTarget(ctx context.Context, targetID string) error {
	if !b.activateBeforeOp {
		return nil
	}
	return activateTarget(ctx, targetID)
}

func (b *Backend) activatePageTargetBeforeAttach(ctx context.Context, devtoolsURL string, targetID string) error {
	if !b.activateBeforeOp {
		return nil
	}
	return activatePageTargetHTTP(ctx, devtoolsURL, targetID)
}

type Remote struct {
	backend *Backend
	cancel  context.CancelFunc
}

func NewRemote(ctx context.Context, devtoolsURL string, allocatorOptions ...chromedp.RemoteAllocatorOption) *Remote {
	runCtx, cancel := context.WithCancel(ctx)
	return &Remote{
		backend: &Backend{
			runCtx:           runCtx,
			devtoolsURL:      devtoolsURL,
			allocatorOptions: append([]chromedp.RemoteAllocatorOption(nil), allocatorOptions...),
		},
		cancel: cancel,
	}
}

func (r *Remote) Navigate(ctx context.Context, navigateURL string) error {
	r.backend.opMu.Lock()
	defer r.backend.opMu.Unlock()

	_, err := withBackendPageTargetContext(r.backend, ctx, r.backend.devtoolsURL, func(targetCtx context.Context, _ pageTargetInfo) (struct{}, error) {
		return struct{}{}, chromedp.Run(targetCtx, chromedp.Navigate(navigateURL))
	})
	return err
}

func (r *Remote) Observe(ctx context.Context, opts api.ObserveOptions) (*api.Observation, error) {
	return r.backend.Observe(ctx, opts)
}

func (r *Remote) Close() {
	r.cancel()
	r.backend.closeRemoteContexts()
}

func (b *Backend) closeRemoteContexts() {
	b.mu.Lock()
	targetCancel := b.targetCancel
	allocCancel := b.allocCancel
	staleContexts := append([]remoteContext(nil), b.staleContexts...)
	b.allocCtx = nil
	b.allocCancel = nil
	b.targetCtx = nil
	b.targetCancel = nil
	b.targetInfo = pageTargetInfo{}
	b.staleContexts = nil
	b.reattachAttempted = false
	b.refLoaderID = ""
	b.refURL = ""
	b.refs = nil
	b.persistentContextID = 0
	b.persistentLoaderID = ""
	b.persistentWorldName = ""
	b.dialogOpen = false
	b.dialogType = ""
	b.dialogMessage = ""
	b.mu.Unlock()

	if targetCancel != nil {
		targetCancel()
	}
	if allocCancel != nil {
		allocCancel()
	}
	cancelRemoteContexts(staleContexts)
}

func cancelRemoteContexts(contexts []remoteContext) {
	for _, current := range contexts {
		if current.targetCancel != nil {
			current.targetCancel()
		}
		if current.allocCancel != nil {
			current.allocCancel()
		}
	}
}

func (b *Backend) trackDialogEvent(targetID string, event any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.targetInfo.ID != "" && b.targetInfo.ID != targetID {
		return
	}
	switch value := event.(type) {
	case *page.EventJavascriptDialogOpening:
		b.dialogOpen = true
		b.dialogType = string(value.Type)
		b.dialogMessage = strings.TrimSpace(value.Message)
	case *page.EventJavascriptDialogClosed:
		b.dialogOpen = false
		b.dialogType = ""
		b.dialogMessage = ""
	}
}

func (b *Backend) javascriptDialogError() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.dialogOpen {
		return nil
	}
	dialogType := b.dialogType
	if dialogType == "" {
		dialogType = "unknown"
	}
	message := b.dialogMessage
	if message == "" {
		message = "(empty message)"
	}
	return fmt.Errorf("screenshot is blocked by an open %s JavaScript dialog: %s", dialogType, message)
}

func operationContext(targetCtx context.Context, requestCtx context.Context) (context.Context, func()) {
	var operationCtx context.Context
	var cancel context.CancelFunc
	if deadline, ok := requestCtx.Deadline(); ok {
		operationCtx, cancel = context.WithDeadline(targetCtx, deadline)
	} else {
		operationCtx, cancel = context.WithCancel(targetCtx)
	}
	stop := context.AfterFunc(requestCtx, cancel)
	return operationCtx, func() {
		stop()
		cancel()
	}
}

func awaitPromise(p *runtime.EvaluateParams) *runtime.EvaluateParams {
	return p.WithAwaitPromise(true)
}

func observeTarget(ctx context.Context, devtoolsURL string, targetInfo pageTargetInfo, opts api.ObserveOptions) (*api.Observation, error) {
	var currentURL string
	var title string
	var text string
	var treeJSON string
	var scopeMeta map[string]string
	var viewportMeta map[string]int
	var loaderID string
	var layoutProperties []string
	if opts.WithLayoutContext {
		layoutProperties = opts.LayoutProperties
	}
	actions := []chromedp.Action{
		chromedp.ActionFunc(func(runCtx context.Context) error {
			frameTree, err := page.GetFrameTree().Do(runCtx)
			if err != nil {
				return err
			}
			if frameTree != nil && frameTree.Frame != nil {
				loaderID = string(frameTree.Frame.LoaderID)
			}
			return nil
		}),
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
		chromedp.Evaluate(`({
			width: window.innerWidth || 0,
			height: window.innerHeight || 0,
			scroll_x: Math.round(window.scrollX || window.pageXOffset || 0),
			scroll_y: Math.round(window.scrollY || window.pageYOffset || 0)
		})`, &viewportMeta),
	}
	if opts.WithText {
		if strings.TrimSpace(opts.ScopeSelector) != "" {
			actions = append(actions, chromedp.Evaluate(scopeTextExpression(opts.ScopeSelector), &text))
		} else {
			actions = append(actions, chromedp.Evaluate(`document.body ? document.body.innerText : ""`, &text))
		}
	}
	if opts.WithTree {
		actions = append(actions, chromedp.Evaluate(observeTreeExpressionWithSelector(opts.CSSProperties, opts.ScopeSelector, layoutProperties, opts.NodeScope, opts.MatchSelector, opts.ExcludeScopeRoot), &treeJSON))
		if strings.TrimSpace(opts.ScopeSelector) != "" {
			actions = append(actions, chromedp.Evaluate(scopeMetaExpression(opts.ScopeSelector), &scopeMeta))
		}
	}
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, err
	}

	tree, err := parseTreeJSON(treeJSON)
	if err != nil {
		return nil, err
	}

	meta := map[string]string{
		"devtools_url":   devtoolsURL,
		"page_target_id": targetInfo.ID,
		"loader_id":      loaderID,
	}
	if viewportMeta["width"] > 0 {
		meta["viewport_width"] = strconv.Itoa(viewportMeta["width"])
	}
	if viewportMeta["height"] > 0 {
		meta["viewport_height"] = strconv.Itoa(viewportMeta["height"])
	}
	meta["scroll_x"] = strconv.Itoa(viewportMeta["scroll_x"])
	meta["scroll_y"] = strconv.Itoa(viewportMeta["scroll_y"])
	for key, value := range scopeMeta {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		meta[key] = strings.TrimSpace(value)
	}

	return &api.Observation{
		URLOrScreen: currentURL,
		Title:       title,
		Text:        strings.TrimSpace(text),
		Tree:        tree,
		Meta:        meta,
	}, nil
}

func (b *Backend) observeViaCDP(ctx context.Context, devtoolsURL string, opts api.ObserveOptions) (*api.Observation, error) {
	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.Observation, error) {
		if opts.WithScreenshot {
			if err := b.javascriptDialogError(); err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(opts.WithinRef) != "" {
			selector, err := b.resolveNodeReferenceInContext(targetCtx, opts.WithinRef)
			if err != nil {
				return nil, err
			}
			opts.ScopeSelector = selector
		}

		startedAt := time.Now()
		observation, err := observeTarget(targetCtx, devtoolsURL, targetInfo, opts)
		if err != nil {
			return nil, err
		}
		observation.Meta["observe_duration_ms"] = strconv.FormatInt(time.Since(startedAt).Milliseconds(), 10)
		if !opts.WithScreenshot {
			if opts.WithTree {
				b.storeObservationReferences(observation)
			}
			return observation, nil
		}

		screenshot, screenshotMeta, err := b.captureScreenshot(ctx, targetCtx, targetInfo, observation, opts)
		if err != nil {
			return nil, err
		}
		observation.ScreenshotData = screenshot
		for key, value := range screenshotMeta {
			observation.Meta[key] = value
		}
		if screenshotMeta["screenshot_recovery"] == "target_replaced" {
			recoveredCtx, recoveredTarget, release, err := b.pageTargetContext(ctx, devtoolsURL)
			if err != nil {
				return nil, err
			}
			recoveredObservation, err := observeTarget(recoveredCtx, devtoolsURL, recoveredTarget, opts)
			release()
			if err != nil {
				return nil, err
			}
			recoveredObservation.ScreenshotData = screenshot
			for key, value := range screenshotMeta {
				recoveredObservation.Meta[key] = value
			}
			observation = recoveredObservation
		}
		if opts.WithTree {
			b.storeObservationReferences(observation)
		}
		return observation, nil
	})
}

func (b *Backend) storeObservationReferences(observation *api.Observation) {
	references := make(map[int]nodeReference, len(observation.Tree))
	for _, node := range observation.Tree {
		if node.ID <= 0 || strings.TrimSpace(node.Selector) == "" {
			continue
		}
		references[node.ID] = nodeReference{
			Selector: node.Selector,
			Identity: stableNodeIdentity(node),
		}
	}
	b.refLoaderID = strings.TrimSpace(observation.Meta["loader_id"])
	b.refURL = strings.TrimSpace(observation.URLOrScreen)
	b.refs = references
}

func (b *Backend) clearObservationReferences() {
	b.refLoaderID = ""
	b.refURL = ""
	b.refs = nil
	b.persistentContextID = 0
	b.persistentLoaderID = ""
	b.persistentWorldName = ""
}

func stableNodeIdentity(node api.Node) string {
	return strings.Join([]string{
		strings.TrimSpace(node.Attrs["tag"]),
		strings.TrimSpace(node.Role),
		strings.TrimSpace(node.Attrs["id"]),
		strings.TrimSpace(node.Attrs["name"]),
		strings.TrimSpace(node.Attrs["data-testid"]),
		strings.TrimSpace(node.Attrs["data-test"]),
		strings.TrimSpace(node.Attrs["aria-label"]),
		strings.TrimSpace(node.Attrs["href"]),
		strings.TrimSpace(node.Attrs["placeholder"]),
	}, "|")
}

func (b *Backend) resolveNodeReference(ctx context.Context, devtoolsURL string, nodeRef string) (string, error) {
	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, _ pageTargetInfo) (string, error) {
		return b.resolveNodeReferenceInContext(targetCtx, nodeRef)
	})
}

func (b *Backend) resolveNodeReferenceInContext(ctx context.Context, nodeRef string) (string, error) {
	nodeID, err := parseNodeReference(nodeRef)
	if err != nil {
		return "", err
	}
	reference, ok := b.refs[nodeID]
	if !ok || strings.TrimSpace(b.refLoaderID) == "" {
		return "", fmt.Errorf("stale node ref %s: run nxctl state or find again", nodeRef)
	}

	var loaderID string
	var currentURL string
	var identity string
	if err := chromedp.Run(
		ctx,
		chromedp.ActionFunc(func(runCtx context.Context) error {
			frameTree, err := page.GetFrameTree().Do(runCtx)
			if err != nil {
				return err
			}
			if frameTree != nil && frameTree.Frame != nil {
				loaderID = string(frameTree.Frame.LoaderID)
			}
			return nil
		}),
		chromedp.Location(&currentURL),
		chromedp.Evaluate(nodeIdentityExpression(reference.Selector), &identity),
	); err != nil {
		return "", err
	}
	if loaderID == "" || loaderID != b.refLoaderID {
		return "", fmt.Errorf("stale node ref %s: page navigated; run nxctl state or find again", nodeRef)
	}
	if strings.TrimSpace(currentURL) != b.refURL {
		return "", fmt.Errorf("stale node ref %s: page URL changed; run nxctl state or find again", nodeRef)
	}
	if identity == "" || identity != reference.Identity {
		return "", fmt.Errorf("stale node ref %s: referenced node changed; run nxctl state or find again", nodeRef)
	}
	return reference.Selector, nil
}

func parseNodeReference(nodeRef string) (int, error) {
	value := strings.TrimSpace(nodeRef)
	if !strings.HasPrefix(value, "@e") {
		return 0, fmt.Errorf("invalid node ref: %s", nodeRef)
	}
	nodeID, err := strconv.Atoi(strings.TrimPrefix(value, "@e"))
	if err != nil || nodeID <= 0 {
		return 0, fmt.Errorf("invalid node ref: %s", nodeRef)
	}
	return nodeID, nil
}

func nodeIdentityExpression(selector string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) return '';
  const roleFor = (element) => {
    const ariaRole = (element.getAttribute('role') || '').trim();
    if (ariaRole) return ariaRole;
    const tag = element.tagName.toLowerCase();
    if (tag === 'a') return 'link';
    if (tag === 'button') return 'button';
    if (tag === 'textarea') return 'textbox';
    if (tag === 'select') return 'combobox';
    if (tag === 'summary') return 'button';
    if (tag === 'input') {
      const type = (element.getAttribute('type') || 'text').toLowerCase();
      if (type === 'checkbox') return 'checkbox';
      if (type === 'radio') return 'radio';
      if (type === 'submit' || type === 'button' || type === 'reset') return 'button';
      return 'textbox';
    }
    if (element.isContentEditable) return 'textbox';
    return tag;
  };
  return [
    el.tagName.toLowerCase(),
    roleFor(el),
    (el.getAttribute('id') || '').trim(),
    (el.getAttribute('name') || '').trim(),
    (el.getAttribute('data-testid') || '').trim(),
    (el.getAttribute('data-test') || '').trim(),
    (el.getAttribute('aria-label') || '').trim(),
    (el.getAttribute('href') || '').trim(),
    (el.getAttribute('placeholder') || '').trim()
  ].join('|');
})()`
}

func (b *Backend) captureScreenshot(requestCtx context.Context, targetCtx context.Context, targetInfo pageTargetInfo, observation *api.Observation, opts api.ObserveOptions) (result []byte, resultMeta map[string]string, resultErr error) {
	meta := map[string]string{
		"screenshot_full": "false",
	}
	if opts.FullScreenshot {
		meta["screenshot_full"] = "true"
	}
	if err := b.javascriptDialogError(); err != nil {
		return nil, nil, err
	}

	totalStartedAt := time.Now()
	captureID := strconv.FormatInt(totalStartedAt.UnixNano(), 10)
	requestTrace := b.newScreenshotTrace(opts.Verbose, captureID, targetInfo.ID, 0)
	requestTrace(fmt.Sprintf(
		"stage=request event=start full=%t recover=%t remaining_ms=%d",
		opts.FullScreenshot,
		opts.RecoverScreenshot,
		contextRemainingMilliseconds(requestCtx),
	))
	defer func() {
		requestTrace(fmt.Sprintf(
			"stage=request event=finish duration_ms=%d error=%q",
			time.Since(totalStartedAt).Milliseconds(),
			errorMessage(resultErr),
		))
	}()

	attemptCtx, cancel := screenshotAttemptContext(targetCtx)
	attemptStartedAt := time.Now()
	attemptTrace := b.newScreenshotTrace(opts.Verbose, captureID, targetInfo.ID, 1)
	attemptTrace(fmt.Sprintf(
		"stage=capture event=start full=%t remaining_ms=%d",
		opts.FullScreenshot,
		contextRemainingMilliseconds(attemptCtx),
	))
	data, width, height, lastErr := captureScreenshotOnce(attemptCtx, opts.FullScreenshot, attemptTrace)
	attemptContextErr := attemptCtx.Err()
	cancel()
	attemptTrace(fmt.Sprintf(
		"stage=capture event=finish duration_ms=%d bytes=%d context_error=%q error=%q",
		time.Since(attemptStartedAt).Milliseconds(),
		len(data),
		errorMessage(attemptContextErr),
		errorMessage(lastErr),
	))
	b.logScreenshotAttempt(targetInfo.ID, "capture", 1, attemptStartedAt, data, lastErr)
	if lastErr == nil {
		return screenshotResult(data, width, height, 1, totalStartedAt, meta)
	}
	if errors.Is(lastErr, errFullScreenshotTooLarge) {
		return nil, nil, lastErr
	}
	if requestCtx.Err() != nil {
		return nil, nil, requestCtx.Err()
	}
	if err := b.javascriptDialogError(); err != nil {
		return nil, nil, err
	}

	reattachStartedAt := time.Now()
	reattachTrace := b.newScreenshotTrace(opts.Verbose, captureID, targetInfo.ID, 0)
	reattachTrace(fmt.Sprintf(
		"stage=reattach event=start remaining_ms=%d",
		contextRemainingMilliseconds(requestCtx),
	))
	reattachedCtx, reattachedTarget, releaseReattached, reattachErr := b.reattachPageTarget(requestCtx, targetInfo, reattachTrace)
	reattachTrace(fmt.Sprintf(
		"stage=reattach event=finish duration_ms=%d error=%q",
		time.Since(reattachStartedAt).Milliseconds(),
		errorMessage(reattachErr),
	))
	b.appendLog(fmt.Sprintf(
		"nexus screenshot target=%s phase=reattach duration_ms=%d error=%q",
		targetInfo.ID,
		time.Since(reattachStartedAt).Milliseconds(),
		errorMessage(reattachErr),
	))
	if reattachErr == nil {
		defer releaseReattached()
		targetCtx = reattachedCtx
		targetInfo = reattachedTarget
		meta["screenshot_recovery"] = "target_reattached"

		attemptCtx, cancel = screenshotAttemptContext(targetCtx)
		attemptStartedAt = time.Now()
		attemptTrace = b.newScreenshotTrace(opts.Verbose, captureID, targetInfo.ID, 2)
		attemptTrace(fmt.Sprintf(
			"stage=capture event=start full=%t remaining_ms=%d",
			opts.FullScreenshot,
			contextRemainingMilliseconds(attemptCtx),
		))
		data, width, height, lastErr = captureScreenshotOnce(attemptCtx, opts.FullScreenshot, attemptTrace)
		attemptContextErr = attemptCtx.Err()
		cancel()
		attemptTrace(fmt.Sprintf(
			"stage=capture event=finish duration_ms=%d bytes=%d context_error=%q error=%q",
			time.Since(attemptStartedAt).Milliseconds(),
			len(data),
			errorMessage(attemptContextErr),
			errorMessage(lastErr),
		))
		b.logScreenshotAttempt(targetInfo.ID, "capture", 2, attemptStartedAt, data, lastErr)
		if lastErr == nil {
			return screenshotResult(data, width, height, 2, totalStartedAt, meta)
		}
		if errors.Is(lastErr, errFullScreenshotTooLarge) {
			return nil, nil, lastErr
		}
		if requestCtx.Err() != nil {
			return nil, nil, requestCtx.Err()
		}
		if err := b.javascriptDialogError(); err != nil {
			return nil, nil, err
		}
	} else {
		lastErr = fmt.Errorf("capture failed and target reattach failed: %w", reattachErr)
	}

	if !opts.RecoverScreenshot {
		return nil, nil, fmt.Errorf("capture screenshot failed after automatic target reattach on target %s: %w", targetInfo.ID, lastErr)
	}

	recoveryStartedAt := time.Now()
	recoveryTrace := b.newScreenshotTrace(opts.Verbose, captureID, targetInfo.ID, 0)
	recoveryTrace(fmt.Sprintf(
		"stage=replace_target event=start remaining_ms=%d",
		contextRemainingMilliseconds(requestCtx),
	))
	recoveryCtx, recoveredTarget, release, err := b.replacePageTarget(
		requestCtx,
		targetCtx,
		observation.URLOrScreen,
		parseMetaInt(observation.Meta, "viewport_width"),
		parseMetaInt(observation.Meta, "viewport_height"),
		parseMetaInt(observation.Meta, "scroll_x"),
		parseMetaInt(observation.Meta, "scroll_y"),
		recoveryTrace,
	)
	if err != nil {
		recoveryTrace(fmt.Sprintf(
			"stage=replace_target event=finish duration_ms=%d error=%q",
			time.Since(recoveryStartedAt).Milliseconds(),
			errorMessage(err),
		))
		return nil, nil, fmt.Errorf("capture screenshot failed and target recovery failed: %w", err)
	}
	defer release()
	recoveryTrace(fmt.Sprintf(
		"stage=replace_target event=finish duration_ms=%d replacement_target=%s error=%q",
		time.Since(recoveryStartedAt).Milliseconds(),
		recoveredTarget.ID,
		"",
	))

	attemptCtx, cancel = screenshotAttemptContext(recoveryCtx)
	attemptStartedAt = time.Now()
	attemptTrace = b.newScreenshotTrace(opts.Verbose, captureID, recoveredTarget.ID, 3)
	attemptTrace(fmt.Sprintf(
		"stage=capture event=start full=%t remaining_ms=%d",
		opts.FullScreenshot,
		contextRemainingMilliseconds(attemptCtx),
	))
	data, width, height, err = captureScreenshotOnce(attemptCtx, opts.FullScreenshot, attemptTrace)
	attemptContextErr = attemptCtx.Err()
	cancel()
	attemptTrace(fmt.Sprintf(
		"stage=capture event=finish duration_ms=%d bytes=%d context_error=%q error=%q",
		time.Since(attemptStartedAt).Milliseconds(),
		len(data),
		errorMessage(attemptContextErr),
		errorMessage(err),
	))
	b.appendLog(fmt.Sprintf(
		"nexus screenshot target=%s phase=recovery duration_ms=%d bytes=%d error=%q",
		recoveredTarget.ID,
		time.Since(recoveryStartedAt).Milliseconds(),
		len(data),
		errorMessage(err),
	))
	if err != nil {
		return nil, nil, fmt.Errorf("capture screenshot failed after replacing target %s with %s: %w", targetInfo.ID, recoveredTarget.ID, err)
	}

	meta["page_target_id"] = recoveredTarget.ID
	meta["screenshot_recovery"] = "target_replaced"
	meta["screenshot_recovery_warning"] = "the unresponsive tab was replaced and transient page state was lost"
	return screenshotResult(data, width, height, 3, totalStartedAt, meta)
}

type screenshotTrace func(string)

func (b *Backend) newScreenshotTrace(verbose bool, captureID string, targetID string, attempt int) screenshotTrace {
	if !verbose {
		return func(string) {}
	}
	prefix := fmt.Sprintf(
		"nexus screenshot capture_id=%s target=%s",
		captureID,
		targetID,
	)
	if attempt > 0 {
		prefix += fmt.Sprintf(" attempt=%d", attempt)
	}
	return func(message string) {
		entry := strings.TrimSpace(prefix + " " + message)
		b.appendLog(entry)
		log.Print(entry)
	}
}

func (b *Backend) logScreenshotAttempt(targetID string, phase string, attempt int, startedAt time.Time, data []byte, err error) {
	b.appendLog(fmt.Sprintf(
		"nexus screenshot target=%s phase=%s attempt=%d duration_ms=%d bytes=%d error=%q",
		targetID,
		phase,
		attempt,
		time.Since(startedAt).Milliseconds(),
		len(data),
		errorMessage(err),
	))
}

func screenshotResult(data []byte, width int64, height int64, attempts int, startedAt time.Time, meta map[string]string) ([]byte, map[string]string, error) {
	meta["screenshot_attempts"] = strconv.Itoa(attempts)
	meta["screenshot_duration_ms"] = strconv.FormatInt(time.Since(startedAt).Milliseconds(), 10)
	if width > 0 {
		meta["screenshot_width"] = strconv.FormatInt(width, 10)
	}
	if height > 0 {
		meta["screenshot_height"] = strconv.FormatInt(height, 10)
	}
	return data, meta, nil
}

func captureScreenshotOnce(ctx context.Context, full bool, trace screenshotTrace) ([]byte, int64, int64, error) {
	var data []byte
	var width int64
	var height int64
	actions := []chromedp.Action{
		screenshotTraceAction(trace, "stage=paint_barrier event=start"),
		chromedp.Evaluate(paintBarrierExpression, nil, awaitPromise),
		screenshotTraceAction(trace, "stage=paint_barrier event=finish"),
	}
	if full {
		actions = append(actions, screenshotTraceAction(trace, "stage=layout_metrics event=start"))
		actions = append(actions, chromedp.ActionFunc(func(runCtx context.Context) error {
			_, _, _, _, _, contentSize, err := page.GetLayoutMetrics().Do(runCtx)
			if err != nil {
				return err
			}
			if contentSize == nil {
				return errors.New("full screenshot content size is unavailable")
			}
			width = int64(contentSize.Width + 0.999999)
			height = int64(contentSize.Height + 0.999999)
			if err := validateFullScreenshotSize(width, height); err != nil {
				return err
			}
			traceScreenshot(trace, fmt.Sprintf(
				"stage=layout_metrics event=finish width=%d height=%d",
				width,
				height,
			))
			return nil
		}))
		actions = append(actions, screenshotTraceAction(trace, "stage=capture_action event=start mode=full"))
		actions = append(actions, chromedp.FullScreenshot(&data, 100))
	} else {
		actions = append(actions, screenshotTraceAction(trace, "stage=capture_action event=start mode=viewport"))
		actions = append(actions, chromedp.CaptureScreenshot(&data))
	}
	actions = append(actions, chromedp.ActionFunc(func(context.Context) error {
		traceScreenshot(trace, fmt.Sprintf("stage=capture_action event=finish bytes=%d", len(data)))
		return nil
	}))
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, width, height, err
	}
	return data, width, height, nil
}

func screenshotTraceAction(trace screenshotTrace, message string) chromedp.Action {
	return chromedp.ActionFunc(func(context.Context) error {
		traceScreenshot(trace, message)
		return nil
	})
}

func traceScreenshot(trace screenshotTrace, message string) {
	if trace != nil {
		trace(message)
	}
}

func contextRemainingMilliseconds(ctx context.Context) int64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return -1
	}
	remaining := time.Until(deadline).Milliseconds()
	if remaining < 0 {
		return 0
	}
	return remaining
}

func validateFullScreenshotSize(width int64, height int64) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid full screenshot dimensions: %dx%d", width, height)
	}
	if width > maxFullScreenshotWidth || height > maxFullScreenshotHeight {
		return fmt.Errorf(
			"%w: page is %dx%d, dimension limits are %dx%d",
			errFullScreenshotTooLarge,
			width,
			height,
			maxFullScreenshotWidth,
			maxFullScreenshotHeight,
		)
	}
	pixels := width * height
	if pixels <= maxFullScreenshotPixels {
		return nil
	}
	return fmt.Errorf(
		"%w: page is %dx%d (%d pixels), pixel limit is %d",
		errFullScreenshotTooLarge,
		width,
		height,
		pixels,
		maxFullScreenshotPixels,
	)
}

func screenshotAttemptContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := screenshotAttemptTimeout
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func (b *Backend) reattachPageTarget(requestCtx context.Context, targetInfo pageTargetInfo, trace screenshotTrace) (context.Context, pageTargetInfo, func(), error) {
	b.mu.Lock()
	runCtx := b.runCtx
	devtoolsURL := b.devtoolsURL
	alreadyAttempted := b.reattachAttempted
	if runCtx != nil && !alreadyAttempted {
		b.reattachAttempted = true
	}
	b.mu.Unlock()
	if runCtx == nil {
		return nil, pageTargetInfo{}, nil, errors.New("chromium backend is not attached")
	}
	if alreadyAttempted {
		return nil, pageTargetInfo{}, nil, errors.New("target reattach was already attempted; use recover-target to replace it")
	}
	traceScreenshot(trace, "stage=reattach_activate_http event=start")
	if err := b.activatePageTargetBeforeAttach(requestCtx, devtoolsURL, targetInfo.ID); err != nil {
		traceScreenshot(trace, fmt.Sprintf("stage=reattach_activate_http event=finish error=%q", errorMessage(err)))
		return nil, pageTargetInfo{}, nil, err
	}
	traceScreenshot(trace, "stage=reattach_activate_http event=finish error=\"\"")

	traceScreenshot(trace, "stage=reattach_context event=start")
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(runCtx, devtoolsURL, b.allocatorOptions...)
	persistentTargetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(targetInfo.ID)))
	chromedp.ListenTarget(persistentTargetCtx, func(event any) {
		b.trackDialogEvent(targetInfo.ID, event)
	})
	traceScreenshot(trace, "stage=reattach_initialize event=start")
	if err := initializePageTargetContext(requestCtx, persistentTargetCtx); err != nil {
		traceScreenshot(trace, fmt.Sprintf("stage=reattach_initialize event=finish error=%q", errorMessage(err)))
		targetCancel()
		allocCancel()
		return nil, pageTargetInfo{}, nil, err
	}
	traceScreenshot(trace, "stage=reattach_initialize event=finish error=\"\"")
	operationCtx, release := operationContext(persistentTargetCtx, requestCtx)
	traceScreenshot(trace, "stage=reattach_activate_cdp event=start")
	if err := b.activatePageTarget(operationCtx, targetInfo.ID); err != nil {
		traceScreenshot(trace, fmt.Sprintf("stage=reattach_activate_cdp event=finish error=%q", errorMessage(err)))
		release()
		b.mu.Lock()
		b.staleContexts = append(b.staleContexts, remoteContext{
			targetCancel: targetCancel,
			allocCancel:  allocCancel,
		})
		b.mu.Unlock()
		return nil, pageTargetInfo{}, nil, err
	}
	traceScreenshot(trace, "stage=reattach_activate_cdp event=finish error=\"\"")

	b.mu.Lock()
	if b.targetInfo.ID != targetInfo.ID {
		b.staleContexts = append(b.staleContexts, remoteContext{
			targetCancel: targetCancel,
			allocCancel:  allocCancel,
		})
		b.mu.Unlock()
		release()
		return nil, pageTargetInfo{}, nil, errors.New("page target changed during reattach")
	}
	b.staleContexts = append(b.staleContexts, remoteContext{
		targetCancel: b.targetCancel,
		allocCancel:  b.allocCancel,
	})
	b.allocCtx = allocCtx
	b.allocCancel = allocCancel
	b.targetCtx = persistentTargetCtx
	b.targetCancel = targetCancel
	b.targetInfo = targetInfo
	b.persistentContextID = 0
	b.persistentLoaderID = ""
	b.persistentWorldName = ""
	b.mu.Unlock()
	traceScreenshot(trace, "stage=reattach_context event=finish error=\"\"")

	return operationCtx, targetInfo, release, nil
}

func (b *Backend) replacePageTarget(requestCtx context.Context, currentTargetCtx context.Context, currentURL string, viewportWidth int, viewportHeight int, scrollX int, scrollY int, trace screenshotTrace) (context.Context, pageTargetInfo, func(), error) {
	var newTargetID target.ID
	traceScreenshot(trace, "stage=create_target event=start")
	createCtx, createCancel := screenshotAttemptContext(currentTargetCtx)
	err := chromedp.Run(createCtx, chromedp.ActionFunc(func(runCtx context.Context) error {
		chromedpContext := chromedp.FromContext(runCtx)
		if chromedpContext == nil || chromedpContext.Browser == nil {
			return errors.New("chromium browser context is unavailable")
		}
		var err error
		newTargetID, err = target.CreateTarget("about:blank").Do(cdp.WithExecutor(runCtx, chromedpContext.Browser))
		return err
	}))
	createCancel()
	if err != nil {
		traceScreenshot(trace, fmt.Sprintf("stage=create_target event=finish error=%q", errorMessage(err)))
		return nil, pageTargetInfo{}, nil, err
	}
	traceScreenshot(trace, fmt.Sprintf("stage=create_target event=finish replacement_target=%s error=%q", newTargetID, ""))

	b.mu.Lock()
	runCtx := b.runCtx
	devtoolsURL := b.devtoolsURL
	oldTargetCancel := b.targetCancel
	oldAllocCancel := b.allocCancel
	staleContexts := append([]remoteContext(nil), b.staleContexts...)
	b.mu.Unlock()
	if runCtx == nil {
		return nil, pageTargetInfo{}, nil, errors.New("chromium backend is not attached")
	}

	traceScreenshot(trace, fmt.Sprintf("stage=attach_replacement event=start replacement_target=%s", newTargetID))
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(runCtx, devtoolsURL, b.allocatorOptions...)
	persistentTargetCtx, targetCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(newTargetID))
	chromedp.ListenTarget(persistentTargetCtx, func(event any) {
		b.trackDialogEvent(string(newTargetID), event)
	})
	traceScreenshot(trace, fmt.Sprintf("stage=initialize_replacement event=start replacement_target=%s", newTargetID))
	if err := initializePageTargetContext(requestCtx, persistentTargetCtx); err != nil {
		traceScreenshot(trace, fmt.Sprintf(
			"stage=initialize_replacement event=finish replacement_target=%s error=%q",
			newTargetID,
			errorMessage(err),
		))
		targetCancel()
		allocCancel()
		return nil, pageTargetInfo{}, nil, err
	}
	traceScreenshot(trace, fmt.Sprintf("stage=initialize_replacement event=finish replacement_target=%s error=%q", newTargetID, ""))
	operationCtx, release := operationContext(persistentTargetCtx, requestCtx)

	actions := []chromedp.Action{
		screenshotTraceAction(trace, fmt.Sprintf("stage=activate_replacement event=start replacement_target=%s", newTargetID)),
		chromedp.ActionFunc(func(runCtx context.Context) error {
			chromedpContext := chromedp.FromContext(runCtx)
			if chromedpContext == nil || chromedpContext.Browser == nil {
				return errors.New("chromium browser context is unavailable")
			}
			if !b.activateBeforeOp {
				return nil
			}
			return target.ActivateTarget(newTargetID).Do(cdp.WithExecutor(runCtx, chromedpContext.Browser))
		}),
		screenshotTraceAction(trace, fmt.Sprintf("stage=activate_replacement event=finish replacement_target=%s", newTargetID)),
	}
	if viewportWidth > 0 && viewportHeight > 0 {
		actions = append(
			actions,
			screenshotTraceAction(trace, fmt.Sprintf(
				"stage=restore_viewport event=start replacement_target=%s width=%d height=%d",
				newTargetID,
				viewportWidth,
				viewportHeight,
			)),
			chromedp.EmulateViewport(int64(viewportWidth), int64(viewportHeight)),
			screenshotTraceAction(trace, fmt.Sprintf("stage=restore_viewport event=finish replacement_target=%s", newTargetID)),
		)
	}
	if strings.TrimSpace(currentURL) != "" {
		actions = append(
			actions,
			screenshotTraceAction(trace, fmt.Sprintf("stage=restore_navigation event=start replacement_target=%s", newTargetID)),
			chromedp.Navigate(currentURL),
			screenshotTraceAction(trace, fmt.Sprintf("stage=restore_navigation event=finish replacement_target=%s", newTargetID)),
		)
	}
	actions = append(
		actions,
		screenshotTraceAction(trace, fmt.Sprintf("stage=restore_paint_barrier event=start replacement_target=%s", newTargetID)),
		chromedp.Evaluate(paintBarrierExpression, nil, awaitPromise),
		screenshotTraceAction(trace, fmt.Sprintf("stage=restore_paint_barrier event=finish replacement_target=%s", newTargetID)),
	)
	if scrollX != 0 || scrollY != 0 {
		actions = append(
			actions,
			screenshotTraceAction(trace, fmt.Sprintf(
				"stage=restore_scroll event=start replacement_target=%s x=%d y=%d",
				newTargetID,
				scrollX,
				scrollY,
			)),
			chromedp.Evaluate(fmt.Sprintf("window.scrollTo(%d, %d)", scrollX, scrollY), nil),
			screenshotTraceAction(trace, fmt.Sprintf("stage=restore_scroll event=finish replacement_target=%s", newTargetID)),
		)
	}
	if err := chromedp.Run(operationCtx, actions...); err != nil {
		traceScreenshot(trace, fmt.Sprintf("stage=attach_replacement event=finish replacement_target=%s error=%q", newTargetID, errorMessage(err)))
		release()
		targetCancel()
		allocCancel()
		return nil, pageTargetInfo{}, nil, err
	}

	targetInfo := pageTargetInfo{
		ID:   string(newTargetID),
		Type: "page",
		URL:  currentURL,
	}
	b.mu.Lock()
	b.allocCtx = allocCtx
	b.allocCancel = allocCancel
	b.targetCtx = persistentTargetCtx
	b.targetCancel = targetCancel
	b.targetInfo = targetInfo
	b.staleContexts = nil
	b.reattachAttempted = false
	b.clearObservationReferences()
	b.dialogOpen = false
	b.dialogType = ""
	b.dialogMessage = ""
	b.mu.Unlock()
	traceScreenshot(trace, fmt.Sprintf("stage=attach_replacement event=finish replacement_target=%s error=%q", newTargetID, ""))

	if oldTargetCancel != nil {
		oldTargetCancel()
	}
	if oldAllocCancel != nil {
		oldAllocCancel()
	}
	cancelRemoteContexts(staleContexts)

	return operationCtx, targetInfo, release, nil
}

func parseMetaInt(meta map[string]string, key string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(meta[key]))
	return value
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

const paintBarrierExpression = `(async () => {
  if (document.readyState === 'loading') {
    await new Promise((resolve) => document.addEventListener('DOMContentLoaded', resolve, {once: true}));
  }
  await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  return document.readyState;
})()`

const hydrationBarrierExpression = `(async () => {
  if (document.readyState === 'loading') {
    await new Promise((resolve) => document.addEventListener('DOMContentLoaded', resolve, {once: true}));
  }
  await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  await new Promise((resolve) => {
    let timer;
    const observer = new MutationObserver(() => {
      clearTimeout(timer);
      timer = setTimeout(() => {
        observer.disconnect();
        resolve();
      }, 100);
    });
    observer.observe(document, {subtree: true, childList: true, attributes: true, characterData: true});
    timer = setTimeout(() => {
      observer.disconnect();
      resolve();
    }, 100);
  });
  await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  return document.readyState;
})()`

func (b *Backend) evalViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("eval script is required")
	}
	world := "main"
	if action.Args != nil && strings.TrimSpace(action.Args["world"]) != "" {
		world = strings.TrimSpace(action.Args["world"])
	}
	if world != "main" && world != "persistent" {
		return nil, errors.New("eval world must be main or persistent")
	}

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		var value interface{}
		evaluateOptions := []chromedp.EvaluateOption{chromedp.EvalAsValue, awaitPromise}
		if world == "persistent" {
			contextID, err := b.persistentEvalContext(targetCtx)
			if err != nil {
				return nil, err
			}
			evaluateOptions = append(evaluateOptions, evalInContext(contextID))
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(evalExpression(action.Text), &value, evaluateOptions...)); err != nil {
			return nil, err
		}

		return &api.ActionResult{
			OK:      true,
			Changed: false,
			Value:   value,
			Meta: map[string]string{
				"devtools_url":   devtoolsURL,
				"page_target_id": targetInfo.ID,
				"eval_world":     world,
			},
		}, nil
	})
}

func (b *Backend) persistentEvalContext(ctx context.Context) (runtime.ExecutionContextID, error) {
	var contextID runtime.ExecutionContextID
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(runCtx context.Context) error {
		frameTree, err := page.GetFrameTree().Do(runCtx)
		if err != nil {
			return err
		}
		if frameTree == nil || frameTree.Frame == nil {
			return errors.New("main frame is unavailable")
		}
		loaderID := string(frameTree.Frame.LoaderID)
		if b.persistentContextID != 0 && loaderID != "" && loaderID == b.persistentLoaderID {
			contextID = b.persistentContextID
			return nil
		}

		if b.persistentWorldName == "" {
			b.persistentWorldName = fmt.Sprintf("nexus-persistent-%d", time.Now().UnixNano())
		}
		contextID, err = page.CreateIsolatedWorld(frameTree.Frame.ID).
			WithWorldName(b.persistentWorldName).
			Do(runCtx)
		if err != nil {
			return err
		}
		b.persistentContextID = contextID
		b.persistentLoaderID = loaderID
		return nil
	}))
	return contextID, err
}

func evalInContext(contextID runtime.ExecutionContextID) chromedp.EvaluateOption {
	return func(params *runtime.EvaluateParams) *runtime.EvaluateParams {
		return params.WithContextID(contextID)
	}
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

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
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
	})
}

func (b *Backend) invokeViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		var (
			message string
			value   map[string]interface{}
		)
		switch {
		case strings.TrimSpace(action.Selector) != "":
			if err := chromedp.Run(targetCtx, chromedp.Evaluate(clickSelectorExpression(action.Selector), &value, chromedp.EvalAsValue)); err != nil {
				return nil, err
			}
			message = fmt.Sprintf("clicked %s", action.Selector)
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
	})
}

func (b *Backend) mouseNodeViaCDP(ctx context.Context, devtoolsURL string, action api.Action, kind string) (*api.ActionResult, error) {
	if action.NodeID == nil || *action.NodeID <= 0 {
		return nil, fmt.Errorf("%s requires a positive index", kind)
	}

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		var point nodePoint
		pointExpression := nodePointExpression(*action.NodeID)
		if strings.TrimSpace(action.Selector) != "" {
			pointExpression = nodePointSelectorExpression(action.Selector)
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(pointExpression, &point, chromedp.EvalAsValue)); err != nil {
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
	})
}

func dispatchKeyEventsWithProbe(ctx context.Context, action chromedp.Action) (int, bool, error) {
	token := fmt.Sprintf("nexus-key-probe-%d", time.Now().UnixNano())
	if err := chromedp.Run(ctx, chromedp.Evaluate(installKeyProbeExpression(token), nil)); err != nil {
		return 0, false, err
	}
	if err := chromedp.Run(ctx, action); err != nil {
		_ = chromedp.Run(ctx, chromedp.Evaluate(finishKeyProbeExpression(token), nil))
		return 0, false, err
	}

	var count int
	if err := chromedp.Run(ctx, chromedp.Evaluate(finishKeyProbeExpression(token), &count)); err != nil {
		return 0, false, nil
	}
	if count < 0 {
		return 0, false, nil
	}
	return count, true, nil
}

func (b *Backend) typeViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("type text is required")
	}

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
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
		markExpression := markTypeTargetExpression(nodeID, token)
		if strings.TrimSpace(action.Selector) != "" {
			markExpression = markTypeTargetSelectorExpression(action.Selector, token)
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(markExpression, &targetValue, chromedp.EvalAsValue)); err != nil {
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
		keydownCount, verified, err := dispatchKeyEventsWithProbe(
			targetCtx,
			chromedp.SendKeys(targetValue.Selector, action.Text, chromedp.ByQuery),
		)
		if err != nil {
			return nil, err
		}
		if verified && keydownCount == 0 {
			if err := b.activatePageTarget(targetCtx, targetInfo.ID); err != nil {
				return nil, err
			}
			keydownCount, verified, err = dispatchKeyEventsWithProbe(
				targetCtx,
				chromedp.SendKeys(targetValue.Selector, action.Text, chromedp.ByQuery),
			)
			if err != nil {
				return nil, err
			}
			if verified && keydownCount == 0 {
				return nil, errors.New("type key events were not delivered to the page")
			}
		}
		value["method"] = "key_events"
		value["keydown_events"] = keydownCount
		value["delivery_verified"] = verified

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
	})
}

func (b *Backend) fillViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.NodeID == nil || *action.NodeID <= 0 {
		return nil, errors.New("fill requires a positive index")
	}
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("fill text is required")
	}

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		var value map[string]interface{}
		expression := typeExpression(*action.NodeID, action.Text)
		if strings.TrimSpace(action.Selector) != "" {
			expression = typeSelectorExpression(action.Selector, action.Text)
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(expression, &value, chromedp.EvalAsValue)); err != nil {
			return nil, err
		}
		value["method"] = "native_value_setter"

		return &api.ActionResult{
			OK:      true,
			Changed: true,
			Message: fmt.Sprintf("filled into %d", *action.NodeID),
			Value:   value,
			Meta: map[string]string{
				"devtools_url":   devtoolsURL,
				"page_target_id": targetInfo.ID,
			},
		}, nil
	})
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

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		keyAction := chromedp.KeyEvent(keyValue, chromedp.KeyModifiers(modifiers...))
		keydownCount, verified, err := dispatchKeyEventsWithProbe(targetCtx, keyAction)
		if err != nil {
			return nil, err
		}
		if verified && keydownCount == 0 {
			if err := b.activatePageTarget(targetCtx, targetInfo.ID); err != nil {
				return nil, err
			}
			keydownCount, verified, err = dispatchKeyEventsWithProbe(targetCtx, keyAction)
			if err != nil {
				return nil, err
			}
			if verified && keydownCount == 0 {
				return nil, errors.New("key event was not delivered to the page")
			}
		}

		return &api.ActionResult{
			OK:      true,
			Changed: true,
			Message: fmt.Sprintf("sent keys %s", keySpec),
			Value: map[string]interface{}{
				"keydown_events":    keydownCount,
				"delivery_verified": verified,
			},
			Meta: map[string]string{
				"devtools_url":   devtoolsURL,
				"page_target_id": targetInfo.ID,
			},
		}, nil
	})
}

func (b *Backend) navigateViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.Args == nil || strings.TrimSpace(action.Args["url"]) == "" {
		return nil, errors.New("navigate url is required")
	}

	navigateURL := strings.TrimSpace(action.Args["url"])
	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		if err := chromedp.Run(targetCtx, chromedp.Navigate(navigateURL)); err != nil {
			return nil, err
		}
		b.clearObservationReferences()

		return &api.ActionResult{
			OK:      true,
			Changed: true,
			Message: fmt.Sprintf("navigated to %s", navigateURL),
			Value: map[string]interface{}{
				"url": navigateURL,
			},
			Meta: map[string]string{
				"devtools_url":   devtoolsURL,
				"page_target_id": targetInfo.ID,
			},
		}, nil
	})
}

func (b *Backend) backViaCDP(ctx context.Context, devtoolsURL string) (*api.ActionResult, error) {
	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(`history.back()`, nil)); err != nil {
			return nil, err
		}
		b.clearObservationReferences()

		return &api.ActionResult{
			OK:      true,
			Changed: true,
			Message: "went back",
			Meta: map[string]string{
				"devtools_url":   devtoolsURL,
				"page_target_id": targetInfo.ID,
			},
		}, nil
	})
}

func (b *Backend) getViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.Args == nil || strings.TrimSpace(action.Args["target"]) == "" {
		return nil, errors.New("get target is required")
	}

	targetKind := strings.TrimSpace(action.Args["target"])
	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
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
		case "text", "value", "attributes":
			if strings.TrimSpace(action.Selector) != "" {
				if err := chromedp.Run(targetCtx, chromedp.Evaluate(getSelectorNodeExpression(targetKind, action.Selector), &value, chromedp.EvalAsValue)); err != nil {
					return nil, err
				}
			} else if action.NodeID == nil || *action.NodeID <= 0 {
				return nil, fmt.Errorf("get %s requires a positive index", targetKind)
			} else {
				if err := chromedp.Run(targetCtx, chromedp.Evaluate(getNodeExpression(targetKind, *action.NodeID), &value, chromedp.EvalAsValue)); err != nil {
					return nil, err
				}
			}
		case "bbox":
			selector := strings.TrimSpace(action.Args["selector"])
			if selector == "" {
				selector = strings.TrimSpace(action.Selector)
			}
			if selector != "" {
				expression := getBBoxExpression(selector)
				if action.Args["scroll_into_view"] == "true" {
					expression = getFocusedBBoxExpression(selector)
				}
				if err := chromedp.Run(targetCtx, chromedp.Evaluate(expression, &value, chromedp.EvalAsValue)); err != nil {
					return nil, err
				}
			} else {
				if action.NodeID == nil || *action.NodeID <= 0 {
					return nil, fmt.Errorf("get %s requires a positive index or selector", targetKind)
				}
				if err := chromedp.Run(targetCtx, chromedp.Evaluate(getNodeExpression(targetKind, *action.NodeID), &value, chromedp.EvalAsValue)); err != nil {
					return nil, err
				}
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
	})
}

func (b *Backend) selectViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.NodeID == nil || *action.NodeID <= 0 {
		return nil, errors.New("select requires a positive index")
	}
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("select value is required")
	}

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		var value map[string]interface{}
		expression := selectExpression(*action.NodeID, action.Text)
		if strings.TrimSpace(action.Selector) != "" {
			expression = selectSelectorExpression(action.Selector, action.Text)
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(expression, &value, chromedp.EvalAsValue)); err != nil {
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
	})
}

func (b *Backend) uploadViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	selector := strings.TrimSpace(action.Selector)
	if selector == "" && (action.NodeID == nil || *action.NodeID <= 0) {
		return nil, errors.New("upload requires a positive index")
	}
	if strings.TrimSpace(action.Text) == "" {
		return nil, errors.New("upload path is required")
	}
	uploadPath, err := filepath.Abs(action.Text)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(uploadPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("upload path must be a regular file")
	}

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		token := fmt.Sprintf("nexus-upload-%d", time.Now().UnixNano())
		defer func() {
			_ = chromedp.Run(targetCtx, chromedp.Evaluate(clearMarkedUploadExpression(token), nil))
		}()
		var marked map[string]interface{}
		markExpression := ""
		targetLabel := selector
		if selector != "" {
			markExpression = markUploadSelectorExpression(selector, token)
		} else {
			markExpression = markUploadNodeExpression(*action.NodeID, token)
			targetLabel = strconv.Itoa(*action.NodeID)
		}
		if err := chromedp.Run(
			targetCtx,
			chromedp.Evaluate(markExpression, &marked, chromedp.EvalAsValue),
			chromedp.SetUploadFiles(`[data-nexus-upload="`+token+`"]`, []string{uploadPath}, chromedp.ByQuery),
		); err != nil {
			return nil, err
		}

		return &api.ActionResult{
			OK:      true,
			Changed: true,
			Message: fmt.Sprintf("uploaded %s to %s", action.Text, targetLabel),
			Value:   marked,
			Meta: map[string]string{
				"devtools_url":   devtoolsURL,
				"page_target_id": targetInfo.ID,
			},
		}, nil
	})
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

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		var value map[string]interface{}
		expression := scrollExpression(nodeID, action.Dir, amount)
		if strings.TrimSpace(action.Selector) != "" {
			expression = scrollSelectorExpression(action.Selector, action.Dir, amount)
		}
		if err := chromedp.Run(targetCtx, chromedp.Evaluate(expression, &value, chromedp.EvalAsValue)); err != nil {
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
	})
}

func (b *Backend) waitViaCDP(ctx context.Context, devtoolsURL string, action api.Action) (*api.ActionResult, error) {
	if action.Args == nil {
		return nil, errors.New("wait target is required")
	}

	targetType := strings.TrimSpace(action.Args["target"])
	value := strings.TrimSpace(action.Args["value"])
	if targetType == "" {
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

	return withBackendPageTargetContext(b, ctx, devtoolsURL, func(targetCtx context.Context, targetInfo pageTargetInfo) (*api.ActionResult, error) {
		var (
			expression string
			waitErr    error
			err        error
		)
		switch targetType {
		case "selector":
			if value == "" {
				return nil, errors.New("wait selector value is required")
			}
			state := strings.TrimSpace(action.Args["state"])
			if state == "" {
				state = "visible"
			}
			expression, err = waitSelectorExpression(value, state)
			if err != nil {
				return nil, err
			}
		case "text":
			if value == "" {
				return nil, errors.New("wait text value is required")
			}
			expression = waitTextExpression(value)
		case "url":
			if value == "" {
				return nil, errors.New("wait url value is required")
			}
			expression = waitURLExpression(value)
		case "navigation":
			waitErr = waitForNavigation(targetCtx, timeout)
		case "hydrated":
			waitErr = waitForHydration(targetCtx, timeout)
		case "function":
			if value == "" {
				return nil, errors.New("wait function value is required")
			}
			waitErr = waitForFunction(targetCtx, value, timeout)
		default:
			return nil, fmt.Errorf("unsupported wait target: %s", targetType)
		}

		if expression != "" {
			waitErr = waitForExpression(targetCtx, expression, timeout)
		}
		if waitErr != nil {
			return nil, waitErr
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
	})
}

func waitForHydration(ctx context.Context, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return chromedp.Run(waitCtx, chromedp.Evaluate(hydrationBarrierExpression, nil, awaitPromise))
}

func waitForNavigation(ctx context.Context, timeout time.Duration) error {
	var initialURL string
	if err := chromedp.Run(ctx, chromedp.Location(&initialURL)); err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for {
		var currentURL string
		err := chromedp.Run(ctx, chromedp.Location(&currentURL))
		if err == nil && currentURL != "" && currentURL != initialURL {
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

func waitForFunction(ctx context.Context, source string, timeout time.Duration) error {
	return waitForExpression(ctx, evalExpression(source), timeout)
}

func waitForExpression(ctx context.Context, expression string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var ready bool
		err := chromedp.Run(ctx, chromedp.Evaluate(expression, &ready, chromedp.EvalAsValue, awaitPromise))
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

func clickSelectorExpression(selector string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  if (el.disabled || el.getAttribute('aria-disabled') === 'true') throw new Error('node is disabled');
  el.scrollIntoView({block: 'center', inline: 'center'});
  el.focus();
  el.click();
  return {
    id: null,
    tag: el.tagName.toLowerCase(),
    text: (el.innerText || el.textContent || '').trim()
  };
})()`
}

func nodePointSelectorExpression(selector string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  el.scrollIntoView({block: 'center', inline: 'center'});
  const rect = el.getBoundingClientRect();
  return {
    id: 0,
    tag: el.tagName.toLowerCase(),
    x: rect.left + rect.width / 2,
    y: rect.top + rect.height / 2
  };
})()`
}

func markTypeTargetExpression(nodeID int, token string) string {
	script := strings.ReplaceAll(markTypeTargetJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$TOKEN$", strconv.Quote(token))
}

func markTypeTargetSelectorExpression(selector string, token string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  const tag = el.tagName.toLowerCase();
  if (!el.isContentEditable && tag !== 'input' && tag !== 'textarea') throw new Error('node is not editable');
  if (el.disabled || el.getAttribute('aria-disabled') === 'true') throw new Error('node is disabled');
  el.scrollIntoView({block: 'center', inline: 'center'});
  el.focus();
  if ((tag === 'input' || tag === 'textarea') && typeof el.setSelectionRange === 'function' && typeof el.value === 'string') {
    try {
      el.setSelectionRange(el.value.length, el.value.length);
    } catch (_) {
    }
  }
  const token = ` + strconv.Quote(token) + `;
  el.setAttribute('data-nexus-type', token);
  return {
    id: null,
    tag,
    selector: '[data-nexus-type="' + token + '"]'
  };
})()`
}

func clearMarkedTypeTargetExpression(token string) string {
	return strings.ReplaceAll(clearMarkedTypeTargetJS, "$TOKEN$", strconv.Quote(token))
}

func installKeyProbeExpression(token string) string {
	return strings.ReplaceAll(installKeyProbeJS, "$TOKEN$", strconv.Quote(token))
}

func finishKeyProbeExpression(token string) string {
	return strings.ReplaceAll(finishKeyProbeJS, "$TOKEN$", strconv.Quote(token))
}

func typeExpression(nodeID int, text string) string {
	script := strings.ReplaceAll(typeNodeJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$TEXT$", strconv.Quote(text))
}

func typeSelectorExpression(selector string, text string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  const tag = el.tagName.toLowerCase();
  if (!el.isContentEditable && tag !== 'input' && tag !== 'textarea') throw new Error('node is not editable');
  if (el.disabled || el.getAttribute('aria-disabled') === 'true') throw new Error('node is disabled');
  const text = ` + strconv.Quote(text) + `;
  el.scrollIntoView({block: 'center', inline: 'center'});
  el.focus();
  if (tag === 'input' || tag === 'textarea') {
    const prototype = tag === 'input' ? window.HTMLInputElement.prototype : window.HTMLTextAreaElement.prototype;
    const valueDescriptor = Object.getOwnPropertyDescriptor(prototype, 'value');
    if (!valueDescriptor || typeof valueDescriptor.set !== 'function') throw new Error('native value setter is unavailable');
    valueDescriptor.set.call(el, text);
    if (typeof el.setSelectionRange === 'function') {
      try {
        el.setSelectionRange(text.length, text.length);
      } catch (_) {
      }
    }
  } else {
    el.textContent = text;
  }
  let inputEvent;
  try {
    inputEvent = new InputEvent('input', {
      bubbles: true,
      composed: true,
      data: text,
      inputType: 'insertReplacementText'
    });
  } catch (_) {
    inputEvent = new Event('input', {bubbles: true, composed: true});
  }
  el.dispatchEvent(inputEvent);
  el.dispatchEvent(new Event('change', {bubbles: true, composed: true}));
  return {id: null, tag, text};
})()`
}

func scrollExpression(nodeID int, dir string, amount int) string {
	script := strings.ReplaceAll(scrollJS, "$NODE_ID$", strconv.Itoa(nodeID))
	script = strings.ReplaceAll(script, "$DIR$", strconv.Quote(dir))
	return strings.ReplaceAll(script, "$AMOUNT$", strconv.Itoa(amount))
}

func scrollSelectorExpression(selector string, dir string, amount int) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  const amount = ` + strconv.Itoa(amount) + `;
  const delta = (` + strconv.Quote(dir) + ` === 'up' ? -1 : 1) *
    (amount > 0 ? amount : Math.max(100, Math.round((el.clientHeight || window.innerHeight) * 0.8)));
  el.scrollTop += delta;
  return {
    scope: 'node',
    id: null,
    dir: ` + strconv.Quote(dir) + `,
    amount: Math.abs(delta),
    top: el.scrollTop
  };
})()`
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

func getBBoxExpression(selector string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) {
    throw new Error('selector not found');
  }
  const rect = el.getBoundingClientRect();
  return {
    x: Math.round(rect.x),
    y: Math.round(rect.y),
    width: Math.round(rect.width),
    height: Math.round(rect.height)
  };
})()`
}

func getFocusedBBoxExpression(selector string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  el.scrollIntoView({block: 'center', inline: 'center'});
  const rect = el.getBoundingClientRect();
  return {
    x: Math.round(rect.x),
    y: Math.round(rect.y),
    w: Math.round(rect.width),
    h: Math.round(rect.height)
  };
})()`
}

func getNodeExpression(kind string, nodeID int) string {
	script := strings.ReplaceAll(getNodeJS, "$KIND$", strconv.Quote(kind))
	return strings.ReplaceAll(script, "$NODE_ID$", strconv.Itoa(nodeID))
}

func getSelectorNodeExpression(kind string, selector string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  const kind = ` + strconv.Quote(kind) + `;
  if (kind === 'text') return (el.innerText || el.textContent || '').trim();
  if (kind === 'value') return 'value' in el ? el.value : '';
  if (kind === 'attributes') {
    return Object.fromEntries(Array.from(el.attributes || []).map((attr) => [attr.name, attr.value]));
  }
  throw new Error('unsupported get target');
})()`
}

func selectExpression(nodeID int, value string) string {
	script := strings.ReplaceAll(selectNodeJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$VALUE$", strconv.Quote(value))
}

func selectSelectorExpression(selector string, value string) string {
	return `(function () {
  const el = document.querySelector(` + strconv.Quote(selector) + `);
  if (!el) throw new Error('selector not found');
  if (el.tagName !== 'SELECT') throw new Error('node is not a select');
  const value = ` + strconv.Quote(value) + `;
  const option = Array.from(el.options).find((item) => item.value === value || item.text === value);
  if (!option) throw new Error('select option not found');
  el.value = option.value;
  el.dispatchEvent(new Event('input', {bubbles: true, composed: true}));
  el.dispatchEvent(new Event('change', {bubbles: true, composed: true}));
  return {id: null, value: el.value, text: option.text};
})()`
}

func markUploadNodeExpression(nodeID int, token string) string {
	script := strings.ReplaceAll(markUploadNodeJS, "$NODE_ID$", strconv.Itoa(nodeID))
	return strings.ReplaceAll(script, "$TOKEN$", strconv.Quote(token))
}

func markUploadSelectorExpression(selector string, token string) string {
	return `(function () {
  const matches = Array.from(document.querySelectorAll(` + strconv.Quote(selector) + `));
  if (matches.length === 0) throw new Error('selector not found');
  if (matches.length > 1) throw new Error('selector matched multiple file inputs');
  const el = matches[0];
  if (el.tagName !== 'INPUT' || (el.getAttribute('type') || '').toLowerCase() !== 'file') {
    throw new Error('node is not a file input');
  }
  const token = ` + strconv.Quote(token) + `;
  el.setAttribute('data-nexus-upload', token);
  return {id: null, selector: '[data-nexus-upload="' + token + '"]'};
})()`
}

func clearMarkedUploadExpression(token string) string {
	return `(function () {
  const el = document.querySelector('[data-nexus-upload="' + ` + strconv.Quote(token) + ` + '"]');
  if (el) el.removeAttribute('data-nexus-upload');
  return true;
})()`
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
	lookupCtx, cancel := context.WithTimeout(ctx, pageTargetTimeout)
	defer cancel()

	var lastErr error
	for {
		target, err := currentPageTargetOnce(lookupCtx, devtoolsURL)
		if err == nil {
			return target, nil
		}
		lastErr = err
		if !errors.Is(err, errPageTargetNotFound) && !isRetryablePageTargetError(err) {
			return pageTargetInfo{}, err
		}

		select {
		case <-lookupCtx.Done():
			if ctx.Err() != nil {
				return pageTargetInfo{}, ctx.Err()
			}
			if lastErr != nil && !errors.Is(lastErr, context.DeadlineExceeded) {
				return pageTargetInfo{}, lastErr
			}
			return pageTargetInfo{}, lookupCtx.Err()
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

func activatePageTargetHTTP(ctx context.Context, devtoolsURL string, targetID string) error {
	activateCtx, cancel := context.WithTimeout(ctx, pageTargetTimeout)
	defer cancel()

	baseURL, err := debugHTTPBaseURL(devtoolsURL)
	if err != nil {
		return err
	}
	requestURL := baseURL + "/json/activate/" + url.PathEscape(strings.TrimSpace(targetID))
	req, err := http.NewRequestWithContext(activateCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if detail := strings.TrimSpace(string(message)); detail != "" {
		return fmt.Errorf("failed to activate page target %s: %s: %s", targetID, resp.Status, detail)
	}
	return fmt.Errorf("failed to activate page target %s: %s", targetID, resp.Status)
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
	ID             int                     `json:"id"`
	Fingerprint    string                  `json:"fingerprint"`
	StructurePath  string                  `json:"structure_path"`
	TextLength     int                     `json:"text_length"`
	Descendants    int                     `json:"descendants"`
	Role           string                  `json:"role"`
	Name           string                  `json:"name"`
	Text           string                  `json:"text"`
	Value          string                  `json:"value"`
	Styles         map[string]string       `json:"styles"`
	LayoutContext  []api.LayoutContextNode `json:"layout_context"`
	Bounds         api.Rect                `json:"bounds"`
	DocumentBounds *api.Rect               `json:"document_bounds"`
	Visible        bool                    `json:"visible"`
	Enabled        bool                    `json:"enabled"`
	Focused        bool                    `json:"focused"`
	Editable       bool                    `json:"editable"`
	Selectable     bool                    `json:"selectable"`
	Invokable      bool                    `json:"invokable"`
	Scrollable     bool                    `json:"scrollable"`
	Children       []int                   `json:"children"`
	Attrs          map[string]string       `json:"attrs"`
	ParentID       *int                    `json:"parent_id"`
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
		parsed := api.Node{
			ID:             node.ID,
			Ref:            formatNodeRef(node.ID),
			Fingerprint:    strings.TrimSpace(node.Fingerprint),
			StructurePath:  strings.TrimSpace(node.StructurePath),
			Selector:       structurePathToSelector(node.StructurePath),
			TextLength:     node.TextLength,
			Descendants:    node.Descendants,
			Role:           node.Role,
			Name:           strings.TrimSpace(node.Name),
			Text:           strings.TrimSpace(node.Text),
			Value:          strings.TrimSpace(node.Value),
			Styles:         node.Styles,
			LayoutContext:  normalizeLayoutContext(node.LayoutContext),
			Bounds:         node.Bounds,
			DocumentBounds: node.DocumentBounds,
			Visible:        node.Visible,
			Enabled:        node.Enabled,
			Focused:        node.Focused,
			Editable:       node.Editable,
			Selectable:     node.Selectable,
			Invokable:      node.Invokable,
			Scrollable:     node.Scrollable,
			Children:       node.Children,
			Attrs:          node.Attrs,
		}
		parsed.LocatorHints = buildLocatorHints(parsed)
		nodes = append(nodes, parsed)
	}

	return nodes, nil
}

func structurePathToSelector(structurePath string) string {
	parts := strings.Split(strings.TrimSpace(structurePath), ">")
	selector := make([]string, 0, len(parts))
	for _, part := range parts {
		tag, ordinal, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok || strings.TrimSpace(tag) == "" {
			return ""
		}
		index, err := strconv.Atoi(strings.TrimSpace(ordinal))
		if err != nil || index <= 0 {
			return ""
		}
		selector = append(selector, fmt.Sprintf("%s:nth-of-type(%d)", strings.TrimSpace(tag), index))
	}
	return strings.Join(selector, " > ")
}

func normalizeLayoutContext(nodes []api.LayoutContextNode) []api.LayoutContextNode {
	if len(nodes) == 0 {
		return nil
	}

	out := make([]api.LayoutContextNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, api.LayoutContextNode{
			Selector:   strings.TrimSpace(node.Selector),
			Role:       strings.TrimSpace(node.Role),
			Name:       strings.TrimSpace(node.Name),
			Styles:     normalizeStringMap(node.Styles),
			Bounds:     node.Bounds,
			Scrollable: node.Scrollable,
			Attrs:      normalizeStringMap(node.Attrs),
		})
	}
	return out
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]string, len(values))
	for key, value := range values {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		out[trimmedKey] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatNodeRef(id int) string {
	return fmt.Sprintf("@e%d", id)
}

func buildLocatorHints(node api.Node) []api.LocatorHint {
	hints := make([]api.LocatorHint, 0, 7)
	seen := map[string]struct{}{}

	add := func(hint api.LocatorHint) {
		hint.Kind = strings.TrimSpace(hint.Kind)
		hint.Value = strings.TrimSpace(hint.Value)
		hint.Name = strings.TrimSpace(hint.Name)
		hint.Command = strings.TrimSpace(hint.Command)
		if hint.Kind == "" || hint.Command == "" {
			return
		}
		key := hint.Kind + "|" + hint.Value + "|" + hint.Name + "|" + hint.Command
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		hints = append(hints, hint)
	}

	role := strings.TrimSpace(node.Role)
	name := preferredLocatorText(node)
	if role != "" && name != "" {
		add(api.LocatorHint{
			Kind:    "role",
			Value:   role,
			Name:    name,
			Command: fmt.Sprintf("role %s --name %s", role, strconv.Quote(name)),
		})
	}

	if text := preferredTextHint(node); text != "" {
		add(api.LocatorHint{
			Kind:    "text",
			Value:   text,
			Command: fmt.Sprintf("text %s", strconv.Quote(text)),
		})
	}

	if (node.Editable || node.Selectable || role == "textbox" || role == "combobox") && strings.TrimSpace(node.Name) != "" {
		label := strings.TrimSpace(node.Name)
		add(api.LocatorHint{
			Kind:    "label",
			Value:   label,
			Command: fmt.Sprintf("label %s", strconv.Quote(label)),
		})
	}

	if ariaLabel := strings.TrimSpace(node.Attrs["aria-label"]); ariaLabel != "" {
		add(api.LocatorHint{
			Kind:    "aria-label",
			Value:   ariaLabel,
			Command: fmt.Sprintf("aria-label %s", strconv.Quote(ariaLabel)),
		})
	}

	if testID := strings.TrimSpace(locatorTestID(node)); testID != "" {
		add(api.LocatorHint{
			Kind:    "testid",
			Value:   testID,
			Command: fmt.Sprintf("testid %s", strconv.Quote(testID)),
		})
	}

	if role == "link" {
		if href := strings.TrimSpace(node.Attrs["href"]); href != "" {
			add(api.LocatorHint{
				Kind:    "href",
				Value:   href,
				Command: fmt.Sprintf("href %s", strconv.Quote(href)),
			})
		}
	}

	if selector := strings.TrimSpace(node.Selector); selector != "" && len(hints) == 0 {
		add(api.LocatorHint{
			Kind:    "css",
			Value:   selector,
			Command: fmt.Sprintf("css %s", strconv.Quote(selector)),
		})
	}

	return hints
}

func preferredLocatorText(node api.Node) string {
	if value := strings.TrimSpace(node.Name); value != "" {
		return value
	}
	if value := strings.TrimSpace(node.Text); value != "" {
		return value
	}
	if value := strings.TrimSpace(node.Attrs["aria-label"]); value != "" {
		return value
	}
	return ""
}

func preferredTextHint(node api.Node) string {
	if value := strings.TrimSpace(node.Text); value != "" {
		return value
	}
	if value := strings.TrimSpace(node.Name); value != "" {
		return value
	}
	return ""
}

func locatorTestID(node api.Node) string {
	if value := strings.TrimSpace(node.Attrs["data-testid"]); value != "" {
		return value
	}
	return strings.TrimSpace(node.Attrs["data-test"])
}
