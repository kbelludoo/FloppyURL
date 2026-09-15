/*
 * LAY Runtime v1.0 — Browser bytecode interpreter for .laybc files.
 *
 * Parses the compact binary format produced by lay_compiler.go and renders
 * a deterministic DOM tree. Zero WASM, zero dependencies, ~3KB minified.
 *
 * Binary format (little-endian):
 *   "LAY1" + version(1) + root(2) + nodeCount(2) + stringCount(2)
 *   string table: [len(2) + bytes] * stringCount
 *   node table:   14 bytes * nodeCount
 *
 * Node record:
 *   parent(2) tag(1) flags(1) text(2) style(2) attrs(2) action(2) childCnt(1) pad(1)
 */
(function () {
  'use strict';

  const TAG_NAMES = {
    0: 'div', 1: 'div', 2: 'div', 3: 'div',
    4: 'h1', 5: 'h2', 6: 'h3', 7: 'p', 8: 'pre',
    9: 'button', 10: 'input', 11: 'span', 12: 'a',
    13: 'img', 14: 'iframe', 15: 'section'
  };

  const TAG_CLASS = {
    0: '', 1: '', 2: 'lay-row', 3: 'lay-col',
    4: '', 5: '', 6: '', 7: '', 8: '',
    9: 'lay-btn', 10: 'lay-input', 11: '', 12: '',
    13: '', 14: '', 15: ''
  };

  function readU16(dv, off) {
    return dv.getUint16(off, true);
  }

  function readString(dv, off, len) {
    let s = '';
    for (let i = 0; i < len; i++) {
      s += String.fromCharCode(dv.getUint8(off + i));
    }
    return s;
  }

  function decodeBytecode(base64) {
    // Convert URL-safe base64 to standard
    let b64 = base64.replace(/-/g, '+').replace(/_/g, '/');
    while (b64.length % 4) b64 += '=';
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return bytes;
  }

  function parseBytecode(bytes) {
    const dv = new DataView(bytes.buffer);
    let off = 0;

    // Magic
    const magic = String.fromCharCode(bytes[0], bytes[1], bytes[2], bytes[3]);
    if (magic !== 'LAY1') {
      throw new Error('Invalid LAY magic: ' + magic);
    }
    off = 4;
    const version = dv.getUint8(off); off++;
    const root = readU16(dv, off); off += 2;
    const nodeCount = readU16(dv, off); off += 2;
    const stringCount = readU16(dv, off); off += 2;

    // String table
    const strings = [];
    for (let i = 0; i < stringCount; i++) {
      const len = readU16(dv, off); off += 2;
      strings.push(readString(dv, off, len));
      off += len;
    }

    // Node table
    const nodes = [];
    for (let i = 0; i < nodeCount; i++) {
      const parent = readU16(dv, off); off += 2;
      const tag = dv.getUint8(off); off++;
      const flags = dv.getUint8(off); off++;
      const textIdx = readU16(dv, off); off += 2;
      const styleIdx = readU16(dv, off); off += 2;
      const attrsIdx = readU16(dv, off); off += 2;
      const actionIdx = readU16(dv, off); off += 2;
      const childCnt = dv.getUint8(off); off++;
      const pad = dv.getUint8(off); off++;

      nodes.push({
        parent: parent === 0xFFFF ? -1 : parent,
        tag: tag,
        text: (flags & 1) ? strings[textIdx] : '',
        style: (flags & 2) ? strings[styleIdx] : '',
        attrs: (flags & 4) ? strings[attrsIdx] : '',
        action: (flags & 8) ? strings[actionIdx] : '',
        childCnt: childCnt
      });
    }

    return { version: version, root: root, nodes: nodes, strings: strings };
  }

  function applyStyle(el, styleStr) {
    if (!styleStr) return;
    const parts = styleStr.split(/\s+/);
    const cssMap = {
      'bg': 'background', 'fg': 'color', 'font': 'font-family',
      'size': 'font-size', 'pad': 'padding', 'margin': 'margin',
      'w': 'width', 'h': 'height', 'display': 'display',
      'border': 'border', 'radius': 'border-radius',
      'align': 'text-align', 'valign': 'align-items',
      'gap': 'gap', 'flex': 'flex-direction', 'weight': 'font-weight',
      'shadow': 'text-shadow', 'cursor': 'cursor', 'opacity': 'opacity'
    };
    for (const part of parts) {
      if (!part) continue;
      const eq = part.indexOf('=');
      if (eq < 0) continue;
      const key = part.substring(0, eq).toLowerCase();
      const val = part.substring(eq + 1);
      const cssProp = cssMap[key] || key;
      el.style[cssProp] = val;
    }
  }

  function applyAttrs(el, attrStr) {
    if (!attrStr) return;
    const parts = attrStr.split(/\s+/);
    for (const part of parts) {
      if (!part) continue;
      const eq = part.indexOf('=');
      if (eq < 0) continue;
      const key = part.substring(0, eq);
      const val = part.substring(eq + 1);
      if (key === 'class') {
        el.className = (el.className ? el.className + ' ' : '') + val;
      } else {
        el.setAttribute(key, val);
      }
    }
  }

  function buildDOM(tree, nodeIdx, parentEl, callbacks) {
    const node = tree.nodes[nodeIdx];
    if (!node) return;

    const tagName = TAG_NAMES[node.tag] || 'div';
    const el = document.createElement(tagName);

    if (TAG_CLASS[node.tag]) {
      el.className = (el.className ? el.className + ' ' : '') + TAG_CLASS[node.tag];
    }

    applyStyle(el, node.style);
    applyAttrs(el, node.attrs);

    if (node.text) {
      el.textContent = node.text;
    }

    if (node.action && callbacks) {
      const action = node.action;
      el.addEventListener('click', function (e) {
        if (callbacks[action]) callbacks[action](el, e);
      });
      if (node.tag === 10) { // input
        el.addEventListener('keypress', function (e) {
          if (e.key === 'Enter' && callbacks[action]) callbacks[action](el, e);
        });
      }
    }

    // Build children
    for (let i = 0; i < tree.nodes.length; i++) {
      if (tree.nodes[i].parent === nodeIdx) {
        buildDOM(tree, i, el, callbacks);
      }
    }

    parentEl.appendChild(el);
  }

  function injectStylesheet() {
    if (document.getElementById('lay-runtime-styles')) return;
    const style = document.createElement('style');
    style.id = 'lay-runtime-styles';
    style.textContent = `
      .lay-row { display: flex; flex-direction: row; gap: 8px; }
      .lay-col { display: flex; flex-direction: column; gap: 8px; }
      .lay-btn { cursor: pointer; }
      .lay-input { background: transparent; border: 1px solid #00ff66; color: #00ff66; padding: 4px 8px; font-family: inherit; }
    `;
    document.head.appendChild(style);
  }

  /**
   * renderLAY(payloadObj, container, callbacks)
   * Renders a LAY payload into the given DOM container.
   * @param {object} payloadObj - {type:"lay", bytecode:"base64...", filename:"..."}
   * @param {HTMLElement} container - target element
   * @param {object} callbacks - map of action names to functions
   * @returns {object} the parsed tree
   */
  window.renderLAY = function (payloadObj, container, callbacks) {
    injectStylesheet();

    const bytecode = decodeBytecode(payloadObj.bytecode || '');
    const tree = parseBytecode(bytecode);

    container.innerHTML = '';
    buildDOM(tree, tree.root, container, callbacks || {});

    return tree;
  };

  /**
   * LAYToFloppy(payloadObj) — renders LAY with default FloppyURL callbacks.
   * Assumes FloppyURL's global functions: printLog, processarHash, inputCampo, etc.
   */
  window.LAYToFloppy = function (payloadObj) {
    const terminal = document.getElementById('console-output');
    if (!terminal) {
      console.error('LAY: no #console-output container found');
      return;
    }

    printLog('> [LAY DSL KERNEL DETECTADO]: ' + (payloadObj.filename || 'app.lay'), 'sucesso');
    printLog('> ARQUITETURA: LAY Bytecode Interpreter (Zero-WASM, Determinístico)', 'alerta');
    printLog('> ATTESTATION ENGINE: NIST FIPS 180-4 SHA-256 via WebCrypto', 'alerta');
    printLog('------------------------------------------------------------------');

    const callbacks = {
      mount: function (el, e) {
        e.preventDefault();
        const input = document.querySelector('[id^="lay-input-"], .lay-input');
        if (input && input.value) {
          processarHash(input.value);
        }
      },
      clear: function (el, e) {
        e.preventDefault();
        processarHash('reboot');
      },
      increment: function (el, e) {
        const counter = document.getElementById('lay-counter');
        if (counter) {
          let n = parseInt(counter.textContent || '0', 10);
          n++;
          counter.textContent = n;
        }
      }
    };

    const tree = window.renderLAY(payloadObj, terminal, callbacks);

    const nodeCount = tree.nodes.length;
    const byteSize = (payloadObj.bytecode || '').length;

    printLog('> LAY TREE MONTADA: ' + nodeCount + ' nós | ' + byteSize + ' bytes bytecode', 'sucesso');
    printLog('> RENDERIZAÇÃO DETERMINÍSTICA DOM CONCLUÍDA.', 'sucesso');

    const inputContainer = document.getElementById('input-container');
    if (inputContainer) inputContainer.style.display = 'none';
  };

})();
