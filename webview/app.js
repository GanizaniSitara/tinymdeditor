// TinyMD page script.
//
// The textarea holds the Markdown and is the only copy of the document. The
// rendered pane is built one top-level block at a time from marked's tokens, each
// block remembering where its source starts and ends. Editing in the rendered pane
// never rewrites Markdown from HTML: every keystroke is intercepted, translated
// through a character map back to source offsets, and applied to the source as a
// splice. Blocks the user did not touch stay byte-for-byte identical.
(function() {
'use strict';

var editor = document.getElementById('editor');
var preview = document.getElementById('preview');
var container = document.getElementById('container');
var savedEl = document.getElementById('saved');
var fnameEl = document.getElementById('fname');
var editorPane = document.querySelector('.editor-pane');
var previewPane = preview;

var useMarked = false;
var editable = false;      // the rendered pane accepts edits (marked loaded, offsets verified)
var blocks = [];           // {start, end, tok, el, text, map}
var pending = null;        // an empty paragraph opened by Enter, not yet in the source
var lastPane = 'editor';   // the pane edit and format commands act on
var savedValue = '';
var lastDirty = false;
var settings = _settings || {};
var view = settings.view || 'split';
var zoom = settings.zoom || 1;
var wide = !!settings.wide;
var lastFind = '';

// ---------------------------------------------------------------- rendering

// Lightweight inline markdown renderer for instant startup (no CDN wait)
function quickMd(s) {
  var h = s
    .replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
    .replace(/^### (.+)$/gm,'<h3>$1</h3>')
    .replace(/^## (.+)$/gm,'<h2>$1</h2>')
    .replace(/^# (.+)$/gm,'<h1>$1</h1>')
    .replace(/\*\*(.+?)\*\*/g,'<strong>$1</strong>')
    .replace(/\*(.+?)\*/g,'<em>$1</em>')
    .replace(/\x60([^\x60]+)\x60/g,'<code>$1</code>')
    .replace(/^[-*] (.+)$/gm,'<li>$1</li>')
    .replace(/^---$/gm,'<hr>')
    .replace(/\n\n/g,'</p><p>')
    .replace(/\n/g,'<br>');
  return '<p>' + h + '</p>';
}

function render() {
  var text = editor.value;
  pending = null;
  if (!useMarked) {
    preview.innerHTML = quickMd(text);
    blocks = [];
    setEditable(false);
    return;
  }
  var toks;
  try { toks = marked.lexer(text); } catch (e) { preview.textContent = String(e); blocks = []; setEditable(false); return; }
  var html = [], pos = 0, ok = true;
  blocks = [];
  for (var k = 0; k < toks.length; k++) {
    var t = toks[k], start = pos;
    pos += t.raw.length;
    if (text.substr(start, t.raw.length) !== t.raw) ok = false;
    if (t.type === 'space') continue;
    var one = [t];
    one.links = toks.links;
    var inner = '';
    try { inner = marked.parser(one); } catch (e) { inner = ''; }
    var fixed = t.type === 'html' || t.type === 'hr';
    html.push('<div class="blk" data-b="' + blocks.length + '"' + (fixed ? ' contenteditable="false"' : '') + '>' + inner + '</div>');
    blocks.push({ start: start, end: pos, tok: t, el: null, text: null, map: null });
  }
  // Only edit when the tokens account for every byte; otherwise offsets could drift.
  if (pos !== text.length) ok = false;
  preview.innerHTML = html.join('');
  for (var b = 0; b < blocks.length; b++) blocks[b].el = preview.children[b];
  // An empty list item cannot hold a caret until it has something in it.
  preview.querySelectorAll('li:empty').forEach(function(li) { li.appendChild(document.createElement('br')); });
  setEditable(ok);
}

function setEditable(on) {
  editable = on;
  preview.contentEditable = on ? 'true' : 'false';
}

// ---------------------------------------------------------------- source map

function childTokens(tok) {
  if (tok.type === 'list') return tok.items;
  if (tok.type === 'table') {
    var out = [];
    (tok.header || []).forEach(function(c) { out = out.concat(c.tokens || []); });
    (tok.rows || []).forEach(function(r) { r.forEach(function(c) { out = out.concat(c.tokens || []); }); });
    return out;
  }
  return tok.tokens || null;
}

var LEAF = { text: 1, escape: 1, codespan: 1 };

// leaves lists the source characters that can appear in a block's rendering, in
// order, with their offsets: the raw text of every text, escape and code token.
// A token is located by its raw text; inside quotes and nested lists marked strips
// the markers first, so there the characters are matched one by one instead.
function leaves(b) {
  var src = editor.value, out = [];
  function greedy(s, cur, end) {
    var p = cur;
    for (var i = 0; i < s.length; i++) {
      for (var k = p; k < end && k < p + 24; k++) {
        if (src[k] === s[i]) { out.push({ c: s[i], p: k }); p = k + 1; break; }
      }
    }
    return p;
  }
  function walk(tok, cur, end) {
    if (typeof tok.raw !== 'string') return cur;
    var idx = src.indexOf(tok.raw, cur);
    var exact = idx >= 0 && idx + tok.raw.length <= end;
    if (tok.type === 'code') {
      var base = exact ? idx : cur, k = tok.raw.indexOf(tok.text);
      if (exact && k >= 0) {
        for (var i = 0; i < tok.text.length; i++) out.push({ c: tok.text[i], p: base + k + i });
        return idx + tok.raw.length;
      }
      return greedy(tok.text, base, end);
    }
    var kids = childTokens(tok);
    if (kids && kids.length) {
      var c = exact ? idx : cur, e = exact ? idx + tok.raw.length : end;
      for (var n = 0; n < kids.length; n++) c = walk(kids[n], c, e);
      return exact ? idx + tok.raw.length : c;
    }
    if (!LEAF[tok.type]) return exact ? idx + tok.raw.length : cur;
    if (exact) {
      for (var m = 0; m < tok.raw.length; m++) out.push({ c: tok.raw[m], p: idx + m });
      return idx + tok.raw.length;
    }
    return greedy(tok.raw, cur, end);
  }
  walk(b.tok, b.start, b.end);
  return out;
}

// blockMap aligns a block's rendered text with its source characters: map[i] is the
// source offset of rendered character i, or -1 for structure such as the line
// break between two list items.
function blockMap(b) {
  if (b.map) return b;
  var T = b.el.textContent, L = leaves(b), map = new Array(T.length), j = 0;
  for (var i = 0; i < T.length; i++) {
    map[i] = -1;
    for (var k = j; k < L.length && k < j + 16; k++) {
      if (L[k].c === T[i]) { map[i] = L[k].p; j = k + 1; break; }
    }
  }
  b.text = T;
  b.map = map;
  return b;
}

// ---------------------------------------------------------------- positions

function elementOf(node) { return node && (node.nodeType === 1 ? node : node.parentNode); }

function textOffset(el, node, off) {
  var r = document.createRange();
  r.setStart(el, 0);
  try { r.setEnd(node, off); } catch (e) { return 0; }
  return r.toString().length;
}

// emptyItemSource finds where text typed into an empty list item belongs: just
// after its marker. The item renders no characters, so the map cannot say.
function emptyItemSource(b, li) {
  var list = li.parentNode;
  if (!list || list.parentNode !== b.el || b.tok.type !== 'list') return -1;
  var index = Array.prototype.indexOf.call(list.children, li), src = editor.value, cur = b.start;
  for (var k = 0; k < b.tok.items.length; k++) {
    var item = b.tok.items[k], at = src.indexOf(item.raw, cur);
    if (at < 0) return -1;
    if (k === index) {
      // marked trims an empty item's raw text, so read the spaces from the source.
      var marker = /^[ \t]*(?:[-*+]|\d{1,9}[.)])/.exec(item.raw), pos = at + (marker ? marker[0].length : 0);
      while (pos < src.length && (src[pos] === ' ' || src[pos] === '\t')) pos++;
      return pos;
    }
    cur = at + item.raw.length;
  }
  return -1;
}

// domToPos turns a DOM point in the rendered pane into {b, i}: block and rendered
// character offset. It carries src directly for the two places with no characters.
function domToPos(node, off) {
  if (!node) return null;
  if (node === preview) {
    var child = preview.childNodes[off] || null;
    if (child && child.classList && child.classList.contains('pending')) return { pending: true };
    if (child && child.dataset && child.dataset.b !== undefined) return { b: +child.dataset.b, i: 0 };
    if (!blocks.length) return null;
    var last = blockMap(blocks[blocks.length - 1]);
    return { b: blocks.length - 1, i: last.text.length };
  }
  var el = elementOf(node);
  if (!el || !preview.contains(el)) return null;
  if (el.closest('.pending')) return { pending: true };
  var blk = el.closest('.blk');
  if (!blk) return null;
  var b = +blk.dataset.b, pos = { b: b, i: textOffset(blk, node, off) };
  var li = el.closest('li');
  if (li && li.textContent.trim() === '') {
    var at = emptyItemSource(blocks[b], li);
    if (at >= 0) pos.src = at;
  }
  return pos;
}

function before(a, c) { return a.b < c.b || (a.b === c.b && a.i < c.i); }

// srcAt is the source offset for a caret: just after the character before it, so
// typing continues the formatting of the text it follows.
function srcAt(p) {
  if (!p) return -1;
  if (p.pending) return pending ? pending.src : -1;
  if (p.src !== undefined) return p.src;
  var b = blockMap(blocks[p.b]), m = b.map, i = p.i;
  if (i > 0 && m[i - 1] >= 0) return m[i - 1] + 1;
  if (i < m.length && m[i] >= 0) return m[i];
  for (var k = i - 1; k >= 0; k--) if (m[k] >= 0) return m[k] + 1;
  for (k = i; k < m.length; k++) if (m[k] >= 0) return m[k];
  return b.start;
}

// nextMapped is the source offset of the first mapped character at or after p.
function nextMapped(p) {
  for (var b = p.b; b < blocks.length; b++) {
    var m = blockMap(blocks[b]).map;
    for (var i = (b === p.b ? p.i : 0); i < m.length; i++) if (m[i] >= 0) return m[i];
  }
  return -1;
}

// pointAt is the DOM point for rendered offset i in block b. Whitespace-only text
// between list items or table cells is skipped, so a caret at a boundary lands at
// the start of the next item rather than in the gap.
function pointAt(b, i) {
  var el = blocks[b].el, walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT), n, acc = 0, last = null;
  while ((n = walker.nextNode())) {
    var len = n.data.length;
    if (i < acc + len || (i === acc + len && !/^\s*$/.test(n.data))) return { node: n, off: i - acc };
    acc += len;
    last = n;
  }
  if (last) return { node: last, off: last.data.length };
  return { node: el, off: 0 };
}

// srcToDom finds the DOM point for a source offset, or null if it lies between blocks.
function srcToDom(C) {
  for (var b = 0; b < blocks.length; b++) {
    var B = blocks[b];
    // marked leaves trailing spaces to a separate token, so they count as this block's.
    if (C < B.start || (C > B.end && !/^[ \t]*$/.test(editor.value.slice(B.end, C)))) continue;
    if (B.tok.type === 'list') {
      var items = B.el.querySelectorAll(':scope > ul > li, :scope > ol > li');
      for (var k = 0; k < items.length; k++) {
        if (items[k].textContent.trim() === '' && emptyItemSource(B, items[k]) === C) return { node: items[k], off: 0 };
      }
    }
    var m = blockMap(B).map, i;
    for (i = 1; i <= m.length; i++) if (m[i - 1] >= 0 && m[i - 1] + 1 === C) return pointAt(b, i);
    for (i = 0; i < m.length; i++) if (m[i] === C) return pointAt(b, i);
    var best = -1, bestD = 1e9;
    for (i = 0; i < m.length; i++) {
      if (m[i] < 0) continue;
      var d = Math.abs(m[i] - C);
      if (d < bestD) { bestD = d; best = m[i] < C ? i + 1 : i; }
    }
    if (best >= 0) return pointAt(b, best);
  }
  return null;
}

// nearestDom is srcToDom with a fallback to the end of the block before the offset.
function nearestDom(C) {
  var p = srcToDom(C);
  if (p) return p;
  for (var b = blocks.length - 1; b >= 0; b--) {
    if (blocks[b].start <= C) return pointAt(b, blockMap(blocks[b]).text.length);
  }
  return blocks.length ? pointAt(0, 0) : { node: preview, off: 0 };
}

function selPositions() {
  var s = window.getSelection();
  if (!s || !s.rangeCount) return null;
  var a = domToPos(s.anchorNode, s.anchorOffset), f = domToPos(s.focusNode, s.focusOffset);
  if (!a || !f) return null;
  return { anchor: a, focus: f, collapsed: s.isCollapsed };
}

function ordered(sp) {
  if (sp.anchor.pending || sp.focus.pending) return [sp.focus, sp.focus];
  return before(sp.focus, sp.anchor) ? [sp.focus, sp.anchor] : [sp.anchor, sp.focus];
}

// ---------------------------------------------------------------- edits

// mappedRanges lists the source behind the rendered characters from a to c. The
// markup between them stays, so deleting part of a bold run or a link leaves its
// markers balanced; markers left around nothing are removed with the text.
function mappedRanges(a, c) {
  var out = [];
  for (var b = a.b; b <= c.b && b < blocks.length; b++) {
    var m = blockMap(blocks[b]).map, from = b === a.b ? a.i : 0, to = b === c.b ? c.i : m.length;
    for (var i = from; i < to; i++) {
      if (m[i] < 0) continue;
      var last = out[out.length - 1];
      if (last && last.e === m[i]) last.e = m[i] + 1;
      else out.push({ s: m[i], e: m[i] + 1 });
    }
  }
  out = out.map(expandEmptyMarkup);
  out.sort(function(x, y) { return x.s - y.s; });
  var merged = [];
  out.forEach(function(r) {
    var last = merged[merged.length - 1];
    if (last && r.s <= last.e) last.e = Math.max(last.e, r.e);
    else merged.push({ s: r.s, e: r.e });
  });
  return merged;
}

function isWordChar(ch) { return /[\p{L}\p{N}_]/u.test(ch || ''); }

// expandEmptyMarkup widens a deletion to take in emphasis or code markers that
// would otherwise be left wrapping nothing, such as the asterisks of **b** when the
// b goes. A marker run only counts if it opens and closes the deleted text.
function expandEmptyMarkup(r) {
  var src = editor.value;
  while (r.s > 0 && r.e < src.length) {
    var c = src[r.s - 1];
    if ('*_~`'.indexOf(c) < 0 || src[r.e] !== c) return r;
    var left = 0, right = 0;
    while (r.s - left - 1 >= 0 && src[r.s - left - 1] === c) left++;
    while (r.e + right < src.length && src[r.e + right] === c) right++;
    var bef = r.s - left - 1, aft = r.e + right;
    if (left !== right || (bef >= 0 && isWordChar(src[bef])) || (aft < src.length && isWordChar(src[aft]))) return r;
    r = { s: r.s - left, e: r.e + right };
  }
  return r;
}

// applyEdits splices edits ({s, e, t}: replace [s, e) with t) into the source as
// one undo step, re-renders, and puts the selection at caret/anchor, which are
// offsets in the edited source.
function applyEdits(edits, caret, anchor, kind) {
  if (!edits.length) return;
  if (anchor === undefined) anchor = caret;
  var v = editor.value, at = Infinity;
  edits.sort(function(x, y) { return (y.s - x.s) || (y.e - x.e); });
  edits.forEach(function(ed) { v = v.slice(0, ed.s) + ed.t + v.slice(ed.e); at = Math.min(at, ed.s); });
  record(v, caret, anchor, at, kind || 'edit');
  editor.value = v;
  render();
  restoreSelection(caret, anchor);
  updateDirty();
}

function restoreSelection(caret, anchor) {
  if (lastPane === 'preview' && editable) {
    var c = nearestDom(caret), a = anchor === caret ? c : nearestDom(anchor);
    var s = window.getSelection();
    try { s.setBaseAndExtent(a.node, a.off, c.node, c.off); } catch (e) {}
    scrollToPoint(c);
  } else {
    var lo = Math.min(caret, anchor), hi = Math.max(caret, anchor);
    editor.setSelectionRange(lo, hi, anchor > caret ? 'backward' : 'forward');
  }
}

function scrollToPoint(p) {
  var el = elementOf(p.node);
  if (el && el.scrollIntoView && preview.contains(el)) el.scrollIntoView({ block: 'nearest' });
}

function insideCell() {
  var s = window.getSelection(), el = s && elementOf(s.focusNode);
  return !!(el && el.closest && el.closest('td,th'));
}

function deleteSelection() {
  var sp = selPositions();
  if (!sp || sp.collapsed) return false;
  var r = ordered(sp), ranges = mappedRanges(r[0], r[1]);
  if (!ranges.length) return false;
  applyEdits(ranges.map(function(x) { return { s: x.s, e: x.e, t: '' }; }), ranges[0].s, undefined, 'delete');
  return true;
}

// typeText replaces the rendered selection with t, or inserts it at the caret.
function typeText(t) {
  if (!editable) return;
  var sp = selPositions();
  if (!sp) return;
  if (sp.focus.pending && pending) {
    var P = pending.src, ins = '\n\n' + t;
    if (t) applyEdits([{ s: P, e: P, t: ins }], P + ins.length, undefined, 'type');
    return;
  }
  if (insideCell()) t = t.replace(/\r?\n/g, ' ').replace(/\|/g, '\\|');
  var edits = [], at;
  if (!sp.collapsed) {
    var r = ordered(sp), ranges = mappedRanges(r[0], r[1]);
    at = ranges.length ? ranges[0].s : srcAt(r[0]);
    ranges.forEach(function(x) { edits.push({ s: x.s, e: x.e, t: '' }); });
  } else {
    at = srcAt(sp.focus);
    // An empty item written as a bare "-" needs a space before its text.
    if (sp.focus.src !== undefined && t && at > 0 && !/[ \t]/.test(editor.value[at - 1])) t = ' ' + t;
  }
  if (at < 0) return;
  if (t) edits.push({ s: at, e: at, t: t });
  applyEdits(edits, at + t.length, undefined, t ? 'type' : 'delete');
}

// deleteRange deletes the rendered characters in a range. With only structure in
// it, such as the break between two list items or two paragraphs, it joins the
// text either side, provided what separates them is just line breaks and markers.
function deleteRange(range) {
  var a = domToPos(range.startContainer, range.startOffset), c = domToPos(range.endContainer, range.endOffset);
  if (!a || !c) return;
  if (a.pending || c.pending) { closePending(); return; }
  var ranges = mappedRanges(a, c);
  if (ranges.length) {
    applyEdits(ranges.map(function(x) { return { s: x.s, e: x.e, t: '' }; }), ranges[0].s, undefined, 'delete');
    return;
  }
  var s = srcAt(a), e = c.src !== undefined ? c.src : nextMapped(c);
  if (s < 0 || e <= s) return;
  var gap = editor.value.slice(s, e);
  if (/^[ \t]*(\n[ \t>]*)+((?:[-*+]|\d{1,9}[.)]|#{1,6})[ \t]+)?[ \t>]*$/.test(gap) && !insideCell()) {
    applyEdits([{ s: s, e: e, t: '' }], s, undefined, 'delete');
  }
}

function deleteDirection(backward, unit) {
  var s = window.getSelection();
  if (!s.rangeCount) return;
  if (!s.isCollapsed) { deleteRange(s.getRangeAt(0)); return; }
  var pos = domToPos(s.focusNode, s.focusOffset);
  if (pos && pos.pending) { closePending(); return; }
  var node = s.focusNode, off = s.focusOffset;
  s.modify('extend', backward ? 'backward' : 'forward', unit || 'character');
  var r = s.rangeCount ? s.getRangeAt(0).cloneRange() : null;
  s.collapse(node, off);
  if (r) deleteRange(r);
}

function openPending(blockIndex, src) {
  var b = blocks[blockIndex];
  if (!b) return;
  var p = document.createElement('p');
  p.className = 'pending';
  p.appendChild(document.createElement('br'));
  b.el.parentNode.insertBefore(p, b.el.nextSibling);
  pending = { el: p, src: src };
  window.getSelection().setBaseAndExtent(p, 0, p, 0);
  p.scrollIntoView({ block: 'nearest' });
}

function closePending() {
  if (!pending) return;
  var src = pending.src;
  pending.el.remove();
  pending = null;
  var c = nearestDom(src);
  window.getSelection().setBaseAndExtent(c.node, c.off, c.node, c.off);
}

function lineStartOf(v, p) { while (p > 0 && v[p - 1] !== '\n') p--; return p; }

// pressEnter splits the rendered text the way its Markdown expects: a new list
// item, a new quoted paragraph, a new line of code, or a new paragraph.
function pressEnter() {
  if (!editable) return;
  var sp = selPositions();
  if (!sp) return;
  if (!sp.collapsed) { deleteSelection(); sp = selPositions(); if (!sp) return; }
  if (sp.focus.pending) return;
  var el = elementOf(window.getSelection().focusNode);
  if (!el || el.closest('td,th')) return;
  var v = editor.value, P = srcAt(sp.focus);
  if (P < 0) return;
  if (el.closest('pre')) {
    var ind = /^[ \t]*/.exec(v.slice(lineStartOf(v, P), P))[0];
    applyEdits([{ s: P, e: P, t: '\n' + ind }], P + 1 + ind.length);
    return;
  }
  var li = el.closest('li');
  if (li) {
    if (li.textContent.trim() === '') {
      // Enter on an empty item ends the list, as in a word processor.
      var ls = lineStartOf(v, P), prevEnd = ls;
      while (prevEnd > 0 && v[prevEnd - 1] === '\n') prevEnd--;
      applyEdits([{ s: prevEnd, e: P, t: '' }], prevEnd);
      var bi = blockAtSource(prevEnd);
      if (bi >= 0) openPending(bi, prevEnd);
      return;
    }
    var marker = null;
    for (var line = lineStartOf(v, P), tries = 0; tries < 50; tries++) {
      marker = /^([ \t]*(?:>[ \t]?)*[ \t]*)([-*+]|\d{1,9})([.)]?)([ \t]+)/.exec(v.slice(line));
      if (marker || line === 0) break;
      line = lineStartOf(v, line - 1);
    }
    var next = '- ';
    if (marker) next = marker[1] + (marker[3] ? String(+marker[2] + 1) : marker[2]) + marker[3] + marker[4];
    applyEdits([{ s: P, e: P, t: '\n' + next }], P + 1 + next.length);
    return;
  }
  if (el.closest('blockquote')) {
    var q = /^[ \t]*(?:>[ \t]?)+/.exec(v.slice(lineStartOf(v, P)));
    q = q ? q[0] : '> ';
    var text = '\n' + q.replace(/[ \t]+$/, '') + '\n' + q;
    applyEdits([{ s: P, e: P, t: text }], P + text.length);
    return;
  }
  var B = blockMap(blocks[sp.focus.b]);
  var atEnd = !B.map.slice(sp.focus.i).some(function(x) { return x >= 0; });
  if (atEnd) { openPending(sp.focus.b, P); return; }
  applyEdits([{ s: P, e: P, t: '\n\n' }], P + 2);
}

function blockAtSource(C) {
  for (var b = 0; b < blocks.length; b++) if (C >= blocks[b].start && C <= blocks[b].end) return b;
  return -1;
}

// ---------------------------------------------------------------- history

// One history for both panes, because rendered edits set the textarea's value and
// that discards the textarea's own undo. Typing within a second is one step.
var hist = [], hi = -1, lastKind = '', lastTime = 0;

function resetHistory() {
  hist = [{ v: editor.value, c: 0, a: 0, at: 0 }];
  hi = 0;
  lastKind = '';
}

function record(v, caret, anchor, at, kind) {
  var now = Date.now();
  if (kind === 'type' && lastKind === 'type' && now - lastTime < 1000 && hi === hist.length - 1 && hi > 0) {
    hist[hi].v = v; hist[hi].c = caret; hist[hi].a = anchor;
  } else {
    hist = hist.slice(0, hi + 1);
    hist.push({ v: v, c: caret, a: anchor, at: at });
    hi = hist.length - 1;
    if (hist.length > 2000) { hist.shift(); hi--; }
  }
  lastKind = kind;
  lastTime = now;
}

function undo() {
  if (hi <= 0) return;
  var undone = hist[hi];
  hi--;
  lastKind = '';
  editor.value = hist[hi].v;
  render();
  restoreSelection(undone.at, undone.at);
  updateDirty();
}

function redo() {
  if (hi >= hist.length - 1) return;
  hi++;
  lastKind = '';
  editor.value = hist[hi].v;
  render();
  restoreSelection(hist[hi].c, hist[hi].a);
  updateDirty();
}

// ---------------------------------------------------------------- formatting

// fmtRange is the source a formatting command applies to: the selection in the
// source pane, or the source behind the rendered selection, or the word at the
// rendered caret when nothing is selected.
function fmtRange() {
  if (lastPane !== 'preview' || !editable) return { s: editor.selectionStart, e: editor.selectionEnd };
  var sp = selPositions();
  if (!sp || sp.focus.pending) return pending ? { s: pending.src, e: pending.src } : null;
  var r = ordered(sp), a = r[0], c = r[1];
  if (sp.collapsed && sp.focus.src === undefined) {
    var T = blockMap(blocks[a.b]).text, ws = a.i, we = a.i;
    while (ws > 0 && isWordChar(T[ws - 1])) ws--;
    while (we < T.length && isWordChar(T[we])) we++;
    if (ws < we) { a = { b: a.b, i: ws }; c = { b: a.b, i: we }; }
  }
  var s = -1, e = -1;
  for (var b = a.b; b <= c.b; b++) {
    var m = blockMap(blocks[b]).map, from = b === a.b ? a.i : 0, to = b === c.b ? c.i : m.length;
    for (var i = from; i < to; i++) if (m[i] >= 0) { if (s < 0) s = m[i]; e = m[i] + 1; }
  }
  if (s >= 0) return { s: s, e: e };
  var at = srcAt(sp.focus);
  return at >= 0 ? { s: at, e: at } : null;
}

// isWrapped reports whether [s, e) is enclosed by exactly these markers. One
// asterisk inside a bold pair is not italic, so the marker run must be the
// marker's own length, or three for bold italic.
function isWrapped(v, s, e, open, close) {
  if (s < open.length || e + close.length > v.length) return false;
  if (v.slice(s - open.length, s) !== open || v.slice(e, e + close.length) !== close) return false;
  if (open !== close) return true;
  var c = open[0], left = 0, right = 0;
  while (s - left - 1 >= 0 && v[s - left - 1] === c) left++;
  while (e + right < v.length && v[e + right] === c) right++;
  return left === right && (left === open.length || (c !== '`' && left === 3));
}

function toggleWrap(open, close, placeholder) {
  var r = fmtRange();
  if (!r) return;
  var v = editor.value, s = r.s, e = r.e;
  if (s === e) {
    if (pending && lastPane === 'preview') {
      var P = pending.src, ins = '\n\n' + open + placeholder + close;
      applyEdits([{ s: P, e: P, t: ins }], P + 2 + open.length + placeholder.length, P + 2 + open.length);
      return;
    }
    applyEdits([{ s: s, e: s, t: open + placeholder + close }], s + open.length + placeholder.length, s + open.length);
    return;
  }
  if (isWrapped(v, s, e, open, close)) {
    applyEdits([{ s: s - open.length, e: s, t: '' }, { s: e, e: e + close.length, t: '' }], e - open.length, s - open.length);
    return;
  }
  applyEdits([{ s: s, e: s, t: open }, { s: e, e: e, t: close }], e + open.length, s + open.length);
}

function fmtCode() {
  var r = fmtRange();
  if (!r) return;
  var v = editor.value;
  if (r.s < r.e && /\n/.test(v.slice(r.s, r.e))) {
    var start = lineStartOf(v, r.s), end = r.e;
    while (end < v.length && v[end] !== '\n') end++;
    applyEdits([{ s: start, e: start, t: '```\n' }, { s: end, e: end, t: '\n```' }], end + 4, start + 4);
    return;
  }
  toggleWrap('`', '`', 'code');
}

async function fmtLink() {
  var r = fmtRange();
  if (!r) return;
  var initial = 'https://';
  try {
    var clip = (await readClipboard() || '').trim();
    if (/^https?:\/\//.test(clip)) initial = clip;
  } catch (e) {}
  var url = await askText('Link address:', initial);
  if (!url || !url.trim()) return;
  var tail = '](' + url.trim() + ')';
  if (r.s === r.e) {
    applyEdits([{ s: r.s, e: r.s, t: '[link text' + tail }], r.s + 10, r.s + 1);
    return;
  }
  applyEdits([{ s: r.s, e: r.s, t: '[' }, { s: r.e, e: r.e, t: tail }], r.e + 1, r.s + 1);
}

// lineStarts lists the source lines from s to e, for the line commands.
function lineStarts(v, s, e) {
  var out = [], line = lineStartOf(v, s);
  for (;;) {
    out.push(line);
    var next = v.indexOf('\n', line);
    if (next < 0 || next + 1 >= e || next + 1 > v.length) break;
    line = next + 1;
  }
  return out;
}

function lineEndOf(v, p) { var n = v.indexOf('\n', p); return n < 0 ? v.length : n; }

var CONTAINER = /^[ \t]*(?:>[ \t]?)*/;
var LISTMARK = /^[ \t]*(?:[-*+]|\d{1,9}[.)])[ \t]+/;

// shiftThrough maps an offset across edits given in the original offsets.
function shiftThrough(pos, edits) {
  var out = pos;
  edits.forEach(function(ed) {
    if (ed.e <= pos) out += ed.t.length - (ed.e - ed.s);
    else if (ed.s < pos) out += ed.s + ed.t.length - pos;
  });
  return out;
}

function lineEdits(edits) {
  if (!edits.length) return;
  var caret, anchor;
  if (lastPane === 'preview' && editable) {
    var sp = selPositions();
    caret = sp ? srcAt(sp.focus) : 0;
    anchor = sp ? srcAt(sp.anchor) : caret;
  } else {
    caret = editor.selectionDirection === 'backward' ? editor.selectionStart : editor.selectionEnd;
    anchor = editor.selectionDirection === 'backward' ? editor.selectionEnd : editor.selectionStart;
  }
  applyEdits(edits.slice(), shiftThrough(caret, edits), shiftThrough(anchor, edits));
}

function fmtHeading() {
  var r = fmtRange();
  if (!r) return;
  var v = editor.value, edits = [];
  lineStarts(v, r.s, Math.max(r.e, r.s + 1)).forEach(function(line) {
    var base = line + CONTAINER.exec(v.slice(line, lineEndOf(v, line)))[0].length;
    var lm = LISTMARK.exec(v.slice(base, lineEndOf(v, base)));
    if (lm) base += lm[0].length;
    var hm = /^(#{1,6})[ \t]+/.exec(v.slice(base, lineEndOf(v, base)));
    if (!hm) edits.push({ s: base, e: base, t: '# ' });
    else if (hm[1].length >= 3) edits.push({ s: base, e: base + hm[0].length, t: '' });
    else edits.push({ s: base, e: base + hm[1].length, t: hm[1] + '#' });
  });
  lineEdits(edits);
}

function fmtList() {
  var r = fmtRange();
  if (!r) return;
  var v = editor.value, found = [], all = true;
  lineStarts(v, r.s, Math.max(r.e, r.s + 1)).forEach(function(line) {
    var base = line + CONTAINER.exec(v.slice(line, lineEndOf(v, line)))[0].length;
    var lm = LISTMARK.exec(v.slice(base, lineEndOf(v, base)));
    if (!lm) all = false;
    found.push({ base: base, end: lm ? base + lm[0].length : -1, blank: base >= lineEndOf(v, base) });
  });
  var edits = [];
  found.forEach(function(f) {
    if (all) edits.push({ s: f.base, e: f.end, t: '' });
    else if (f.end < 0 && !f.blank) edits.push({ s: f.base, e: f.base, t: '- ' });
  });
  lineEdits(edits);
}

// ---------------------------------------------------------------- find

function findNext() {
  if (!lastFind) { findPrompt(); return; }
  var needle = lastFind.toLowerCase(), found = false;
  if (lastPane === 'preview' && blocks.length) {
    var sp = selPositions(), start = sp ? ordered(sp)[1] : { b: 0, i: 0 };
    if (start.pending) start = { b: 0, i: 0 };
    for (var k = 0; k <= blocks.length && !found; k++) {
      var b = (start.b + k) % blocks.length, T = blockMap(blocks[b]).text.toLowerCase();
      var from = k === 0 ? start.i : 0, idx = T.indexOf(needle, from);
      if (k === blocks.length) idx = T.indexOf(needle) < start.i ? T.indexOf(needle) : -1;
      if (idx >= 0) {
        var a = pointAt(b, idx), c = pointAt(b, idx + needle.length);
        window.getSelection().setBaseAndExtent(a.node, a.off, c.node, c.off);
        scrollToPoint(c);
        preview.focus();
        found = true;
      }
    }
  } else {
    var v = editor.value.toLowerCase(), at = v.indexOf(needle, editor.selectionEnd);
    if (at < 0) at = v.indexOf(needle);
    if (at >= 0) {
      editor.focus();
      editor.setSelectionRange(at, at + needle.length);
      found = true;
    }
  }
  if (!found) flashStatus('Not found');
  return found;
}

async function findPrompt() {
  var text = await askText('Find:', lastFind);
  if (text === null || text === '') return;
  lastFind = text;
  findNext();
}

// askText shows a one-line prompt in the toolbar; it resolves to null on Escape.
var askResolve = null;
function askText(label, initial) {
  var box = document.getElementById('ask'), input = document.getElementById('askInput');
  document.getElementById('askLabel').textContent = label;
  input.value = initial || '';
  box.classList.add('show');
  input.focus();
  input.select();
  return new Promise(function(resolve) { askResolve = resolve; });
}
function finishAsk(value) {
  var box = document.getElementById('ask');
  box.classList.remove('show');
  var resolve = askResolve;
  askResolve = null;
  focusPane();
  if (resolve) resolve(value);
}
document.getElementById('askInput').addEventListener('keydown', function(e) {
  if (e.key === 'Enter') { e.preventDefault(); finishAsk(this.value); }
  else if (e.key === 'Escape') { e.preventDefault(); finishAsk(null); }
});
document.getElementById('askInput').addEventListener('blur', function() {
  if (askResolve) setTimeout(function() { if (askResolve && document.activeElement !== document.getElementById('askInput')) finishAsk(null); }, 0);
});

// ---------------------------------------------------------------- views

function applyView(next, focus) {
  if (next === 'rendered' && lastPane !== 'preview') syncPreviewFromEditor();
  if (next === 'source' && lastPane === 'preview') syncEditorFromPreview();
  view = next;
  container.classList.remove('view-source', 'view-rendered', 'view-split');
  container.classList.add('view-' + view);
  document.querySelectorAll('.views button').forEach(function(btn) {
    btn.classList.toggle('on', btn.dataset.view === view);
  });
  if (view === 'rendered') lastPane = 'preview';
  if (view === 'source') lastPane = 'editor';
  if (focus !== false) focusPane();
  saveSettings();
}

function focusPane() {
  if (lastPane === 'preview' && view !== 'source') preview.focus();
  else editor.focus();
}

// The caret moves with the view, so switching panes keeps your place.
function syncPreviewFromEditor() {
  if (!editable) return;
  var c = nearestDom(editor.selectionEnd), a = nearestDom(editor.selectionStart);
  try { window.getSelection().setBaseAndExtent(a.node, a.off, c.node, c.off); } catch (e) {}
  setTimeout(function() { scrollToPoint(c); }, 0);
}

function syncEditorFromPreview() {
  var sp = selPositions();
  if (!sp) return;
  var c = srcAt(sp.focus), a = srcAt(sp.anchor);
  if (c >= 0 && a >= 0) editor.setSelectionRange(Math.min(a, c), Math.max(a, c));
}

function setZoom(z) {
  zoom = Math.max(0.6, Math.min(3, Math.round(z * 10) / 10));
  document.documentElement.style.setProperty('--z', zoom);
  saveSettings();
}

function setWide(on) {
  wide = on;
  document.body.classList.toggle('wide', wide);
  saveSettings();
}

function saveSettings() {
  if (typeof goSaveSettings === 'function') {
    goSaveSettings(JSON.stringify({ view: view, zoom: zoom, wide: wide, split: splitPct }));
  }
}

// ---------------------------------------------------------------- files

function flashStatus(text) {
  savedEl.textContent = text || 'Saved!';
  savedEl.classList.add('show');
  setTimeout(function() { savedEl.classList.remove('show'); savedEl.textContent = 'Saved!'; }, 1500);
}

function updateDirty() {
  var dirty = editor.value !== savedValue;
  if (dirty !== lastDirty) {
    lastDirty = dirty;
    if (typeof goSetDirty === 'function') goSetDirty(dirty);
  }
}

function markSaved() {
  savedValue = editor.value;
  updateDirty();
}

function loadDocument(content, name) {
  editor.value = content;
  if (name !== undefined) fnameEl.textContent = name;
  render();
  resetHistory();
  markSaved();
  editor.setSelectionRange(0, 0);
  editor.scrollTop = 0;
  preview.scrollTop = 0;
  if (lastPane === 'preview' && blocks.length) {
    var p = pointAt(0, 0);
    try { window.getSelection().setBaseAndExtent(p.node, p.off, p.node, p.off); } catch (e) {}
  }
  focusPane();
}

// confirmDiscard offers to save unsaved changes; false means stay put.
async function confirmDiscard() {
  if (editor.value === savedValue || typeof goConfirmDiscard !== 'function') return true;
  var answer = await goConfirmDiscard();
  if (answer === 'yes') return await saveDocument();
  return answer === 'no';
}

async function newDocument() {
  if (!(await confirmDiscard())) return;
  await goNewFile();
  loadDocument('', 'Untitled');
}

async function openDocument() {
  if (!(await confirmDiscard())) return;
  var opened = await goOpenFile();
  if (opened && opened.content !== undefined) {
    loadDocument(opened.content, opened.name);
    flashStatus('Opened');
  } else if (opened && opened.error) {
    flashStatus(opened.error);
  }
}

async function openRecent(index) {
  if (!(await confirmDiscard())) return;
  var opened = await goOpenRecent(index);
  if (opened && opened.content !== undefined) loadDocument(opened.content, opened.name);
  else if (opened && opened.error) flashStatus(opened.error);
}

async function saveDocumentAs() {
  var saveAsName = await goSaveFileAs(editor.value);
  if (saveAsName && saveAsName.indexOf('error:') !== 0) {
    fnameEl.textContent = saveAsName;
    markSaved();
    flashStatus('Saved!');
    return true;
  }
  if (saveAsName) flashStatus(saveAsName);
  return false;
}

async function saveDocument() {
  var result = await goSaveFile(editor.value);
  if (result === 'ok') {
    markSaved();
    flashStatus('Saved!');
    return true;
  }
  if (result === 'no file') return await saveDocumentAs();
  flashStatus(result);
  return false;
}

async function revealDocument() {
  var reveal = await goRevealFile();
  if (reveal !== 'ok') flashStatus('No file');
}

// ---------------------------------------------------------------- clipboard

async function readClipboard() {
  try {
    if (typeof goReadClipboard === 'function') {
      var nativeResult = await goReadClipboard();
      if (nativeResult && nativeResult.text !== undefined) return nativeResult.text;
    }
  } catch (_) {}
  try {
    if (navigator.clipboard && navigator.clipboard.readText) return await navigator.clipboard.readText();
  } catch (_) {}
  return null;
}

async function writeClipboard(text) {
  try {
    if (typeof goWriteClipboard === 'function') {
      var nativeResult = await goWriteClipboard(text);
      if (nativeResult === 'ok') return true;
    }
  } catch (_) {}
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) { await navigator.clipboard.writeText(text); return true; }
  } catch (_) {}
  return false;
}

function selectedEditorText() {
  return editor.value.substring(editor.selectionStart || 0, editor.selectionEnd || 0);
}

function replaceEditorSelection(text) {
  var start = editor.selectionStart || 0, end = editor.selectionEnd || 0;
  applyEditorEdit(start, end, text);
}

function applyEditorEdit(start, end, text) {
  var v = editor.value.slice(0, start) + text + editor.value.slice(end);
  record(v, start + text.length, start + text.length, start, 'edit');
  editor.value = v;
  editor.setSelectionRange(start + text.length, start + text.length);
  render();
  updateDirty();
}

// handleEditorShortcut runs Ctrl+A, C, V and X, which arrive from the native
// keyboard hook because WebView2 does not deliver them reliably. The rendered
// pane, the toolbar prompt and the source pane each get their own behaviour.
async function handleEditorShortcut(key, force) {
  var active = document.activeElement;
  if (active && active.id === 'askInput') {
    if (key === 'a') { active.select(); return true; }
    if (key === 'v') { var t = await readClipboard(); if (t) active.setRangeText(t.replace(/\r?\n/g, ' '), active.selectionStart, active.selectionEnd, 'end'); return true; }
    if (key === 'c' || key === 'x') {
      var part = active.value.slice(active.selectionStart, active.selectionEnd);
      if (part) await writeClipboard(part);
      if (key === 'x') active.setRangeText('', active.selectionStart, active.selectionEnd, 'end');
      return true;
    }
    return false;
  }
  if (lastPane === 'preview' && view !== 'source') {
    var s = window.getSelection();
    if (key === 'a') {
      preview.focus();
      var r = document.createRange();
      r.selectNodeContents(preview);
      s.removeAllRanges();
      s.addRange(r);
      return true;
    }
    if (key === 'c' || key === 'x') {
      var text = s ? s.toString() : '';
      if (!text) return false;
      await writeClipboard(text);
      if (key === 'x') deleteSelection();
      return true;
    }
    if (key === 'v') {
      var paste = await readClipboard();
      if (paste) typeText(paste.replace(/\r\n?/g, '\n'));
      return true;
    }
    return false;
  }
  if (key === 'a') { editor.focus(); editor.select(); return true; }
  if (key === 'x') {
    var cutText = selectedEditorText();
    if (!cutText) return false;
    await writeClipboard(cutText);
    replaceEditorSelection('');
    return true;
  }
  if (key === 'c') {
    var copyText = selectedEditorText();
    if (!copyText) return false;
    await writeClipboard(copyText);
    return true;
  }
  if (key === 'v') {
    editor.focus();
    var pasteText = await readClipboard();
    if (pasteText !== null && pasteText !== undefined) replaceEditorSelection(pasteText);
    return true;
  }
  return false;
}
window.tinyMdHandleEditorShortcut = handleEditorShortcut;

// ---------------------------------------------------------------- commands

var commands = {
  'new': newDocument,
  open: openDocument,
  save: saveDocument,
  saveAs: saveDocumentAs,
  print: function() { window.print(); },
  reveal: revealDocument,
  undo: undo,
  redo: redo,
  cut: function() { return handleEditorShortcut('x', true); },
  copy: function() { return handleEditorShortcut('c', true); },
  paste: function() { return handleEditorShortcut('v', true); },
  selectAll: function() { return handleEditorShortcut('a', true); },
  find: findPrompt,
  findNext: findNext,
  viewSource: function() { applyView('source'); },
  viewRendered: function() { applyView('rendered'); },
  split: function() { applyView('split'); },
  zoomIn: function() { setZoom(zoom + 0.1); },
  zoomOut: function() { setZoom(zoom - 0.1); },
  zoomReset: function() { setZoom(1); },
  wide: function() { setWide(!wide); },
  bold: function() { toggleWrap('**', '**', 'bold text'); },
  italic: function() { toggleWrap('*', '*', 'italic text'); },
  heading: fmtHeading,
  list: fmtList,
  link: fmtLink,
  code: fmtCode,
  recent: openRecent,
  saveThenClose: async function() { if (await saveDocument()) goCloseWindow(); },
};

window.tinyMdMenuCommand = async function(command, arg) {
  var fn = commands[command];
  if (fn) return fn(arg);
};

// Toolbar buttons act on the pane in use, so they must not take the focus or the
// selection from it.
document.getElementById('toolbar').addEventListener('mousedown', function(e) {
  if (e.target.closest('button')) e.preventDefault();
});
document.getElementById('toolbar').addEventListener('click', function(e) {
  var btn = e.target.closest('button');
  if (btn && commands[btn.dataset.cmd]) commands[btn.dataset.cmd]();
});

var SHORTCUTS = {
  'n': 'new', 'o': 'open', 's': 'save', 'S': 'saveAs', 'e': 'reveal', 'p': 'print',
  'f': 'find', 'z': 'undo', 'Z': 'redo', 'y': 'redo', 'b': 'bold', 'i': 'italic', 'k': 'link',
  'H': 'heading', 'L': 'list', 'K': 'code',
  // The view switch reads left to right: Markdown, Split, Rendered.
  '1': 'viewSource', '2': 'split', '3': 'viewRendered', '0': 'split',
  '=': 'zoomIn', '+': 'zoomIn', '-': 'zoomOut',
};

document.addEventListener('keydown', async function(e) {
  if (e.key === 'F3') { e.preventDefault(); findNext(); return; }
  if (!e.ctrlKey || e.altKey || e.metaKey) return;
  var key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
  if (e.shiftKey && /^[a-z]$/.test(key)) key = key.toUpperCase();
  if (key === 'C') { e.preventDefault(); if (typeof goCopyPath === 'function') flashStatus(await goCopyPath()); return; }
  var name = SHORTCUTS[key];
  if (!name) return;
  if (document.activeElement && document.activeElement.id === 'askInput' && (name === 'undo' || name === 'redo')) return;
  e.preventDefault();
  e.stopPropagation();
  commands[name]();
}, true);

document.addEventListener('wheel', function(e) {
  if (!e.ctrlKey) return;
  e.preventDefault();
  setZoom(zoom + (e.deltaY < 0 ? 0.1 : -0.1));
}, { passive: false });

// ---------------------------------------------------------------- rendered input

preview.addEventListener('beforeinput', function(e) {
  if (!editable) { e.preventDefault(); return; }
  var t = e.inputType;
  if (t === 'insertCompositionText') return; // an IME owns this; the input handler re-syncs
  e.preventDefault();
  if (t === 'insertText' || t === 'insertReplacementText') {
    var data = e.data;
    if (data == null && e.dataTransfer) data = e.dataTransfer.getData('text/plain');
    if (data != null) typeText(data);
  } else if (t === 'insertParagraph') {
    pressEnter();
  } else if (t === 'insertLineBreak') {
    if (!insideCell()) typeText('\n');
  } else if (t === 'insertFromPaste' || t === 'insertFromDrop') {
    var text = e.dataTransfer ? e.dataTransfer.getData('text/plain') : '';
    if (text) typeText(text.replace(/\r\n?/g, '\n'));
  } else if (t.indexOf('delete') === 0) {
    var ranges = e.getTargetRanges ? e.getTargetRanges() : [];
    if (ranges.length) deleteRange(ranges[0]);
    else deleteDirection(t.indexOf('Backward') >= 0, t.indexOf('Word') >= 0 ? 'word' : 'character');
  } else if (t === 'historyUndo') {
    undo();
  } else if (t === 'historyRedo') {
    redo();
  } else if (t === 'formatBold') {
    commands.bold();
  } else if (t === 'formatItalic') {
    commands.italic();
  }
});

// After an IME composes text straight into the DOM, rebuild from the source so the
// rendering never disagrees with the document.
preview.addEventListener('input', function(e) {
  if (e.inputType === 'insertCompositionText' || e.isComposing) return;
  if (editable) render();
});
preview.addEventListener('compositionend', function(e) {
  if (!editable) return;
  render();
  if (e.data) typeText(e.data);
});

preview.addEventListener('copy', function(e) {
  var selection = window.getSelection();
  var text = selection ? selection.toString() : '';
  if (!text) return;
  if (e.clipboardData) e.clipboardData.setData('text/plain', text);
  e.preventDefault();
  writeClipboard(text);
});
preview.addEventListener('cut', function(e) {
  e.preventDefault();
  handleEditorShortcut('x', true);
});
preview.addEventListener('paste', function(e) {
  e.preventDefault();
  var text = e.clipboardData ? e.clipboardData.getData('text/plain') : '';
  if (text) typeText(text.replace(/\r\n?/g, '\n'));
});

// Clicking a link in the rendered pane places the caret; Ctrl+click follows it.
preview.addEventListener('click', function(e) {
  var a = e.target.closest && e.target.closest('a');
  if (!a) return;
  e.preventDefault();
});

// ---------------------------------------------------------------- source pane

editor.addEventListener('input', function(e) {
  var kind = e.inputType === 'insertText' ? 'type' : 'edit';
  record(editor.value, editor.selectionEnd, editor.selectionStart, editor.selectionStart, kind);
  render();
  updateDirty();
});

editor.addEventListener('copy', function(e) {
  var text = selectedEditorText();
  if (!text || !e.clipboardData) return;
  e.clipboardData.setData('text/plain', text);
  e.preventDefault();
});

editor.addEventListener('paste', function(e) {
  if (!e.clipboardData) return;
  var text = e.clipboardData.getData('text/plain');
  if (text === undefined || text === null) return;
  e.preventDefault();
  replaceEditorSelection(text);
});

editor.addEventListener('keydown', function(e) {
  if (e.key === 'Tab') {
    e.preventDefault();
    replaceEditorSelection('\t');
  }
});

editor.addEventListener('focus', function() { lastPane = 'editor'; });
preview.addEventListener('focus', function() { lastPane = 'preview'; });
editorPane.addEventListener('mousedown', function(e) {
  lastPane = 'editor';
  if (e.target !== editor) editor.focus();
});
preview.addEventListener('mousedown', function() { lastPane = 'preview'; });

// ---------------------------------------------------------------- divider

var splitPct = settings.split || 50;
(function() {
  var divider = document.getElementById('divider');
  var dragging = false;
  function apply() {
    editorPane.style.flexBasis = splitPct + '%';
    previewPane.style.flexBasis = (100 - splitPct) + '%';
  }
  apply();
  divider.addEventListener('mousedown', function(e) {
    e.preventDefault();
    dragging = true;
    divider.classList.add('active');
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  });
  document.addEventListener('mousemove', function(e) {
    if (!dragging) return;
    var rect = container.getBoundingClientRect();
    var tree = document.getElementById('tree');
    var left = rect.left + (tree.classList.contains('hidden') ? 0 : tree.offsetWidth);
    splitPct = Math.max(15, Math.min(85, ((e.clientX - left) / (rect.right - left)) * 100));
    apply();
  });
  document.addEventListener('mouseup', function() {
    if (!dragging) return;
    dragging = false;
    divider.classList.remove('active');
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    saveSettings();
  });
})();

// ---------------------------------------------------------------- tree (browse mode)

(function() {
  if (!_tree) return;
  var pane = document.getElementById('tree');
  pane.classList.remove('hidden');
  var activeEl = null;

  function buildList(node) {
    var ul = document.createElement('ul');
    (node.children || []).forEach(function(c) {
      var li = document.createElement('li');
      var row = document.createElement('div');
      row.className = 'node ' + (c.dir ? 'dir' : 'file');
      if (c.dir) {
        row.innerHTML = '<span class="caret">v</span>' + escapeHtml(c.name);
        li.appendChild(row);
        var sub = buildList(c);
        li.appendChild(sub);
        row.addEventListener('click', function() {
          var hidden = sub.style.display === 'none';
          sub.style.display = hidden ? '' : 'none';
          row.firstChild.textContent = hidden ? 'v' : '>';
        });
      } else {
        row.textContent = c.name;
        row.dataset.path = c.path;
        row.addEventListener('click', async function() {
          if (!(await confirmDiscard())) return;
          var res = await goLoadFile(c.path);
          if (res && res.content !== undefined) {
            loadDocument(res.content, res.name);
            if (activeEl) activeEl.classList.remove('active');
            row.classList.add('active');
            activeEl = row;
          }
        });
        li.appendChild(row);
      }
      ul.appendChild(li);
    });
    return ul;
  }
  function escapeHtml(s) {
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }
  var header = document.createElement('div');
  header.className = 'node dir';
  header.style.fontSize = '11px';
  header.style.color = '#888';
  header.style.padding = '2px 12px 6px';
  header.textContent = _tree.name;
  pane.appendChild(header);
  pane.appendChild(buildList(_tree));
})();

// ---------------------------------------------------------------- start up

if (_initContent) editor.value = _initContent;
if (_initFname) fnameEl.textContent = _initFname.replace(/.*[\\\/]/, '');
savedValue = editor.value;
resetHistory();
document.documentElement.style.setProperty('--z', zoom);
document.body.classList.toggle('wide', wide);
render();
applyView(view, false);
if (view === 'rendered') lastPane = 'preview';
focusPane();

// marked.js is compiled into the executable, so the full renderer is there from
// the first paint and needs no network. The quick renderer remains as a fallback.
if (window.marked) {
  marked.setOptions({ breaks: true, gfm: true });
  useMarked = true;
  render();
  if (lastPane === 'preview' && blocks.length) {
    var p = nearestDom(editor.selectionStart);
    try { window.getSelection().setBaseAndExtent(p.node, p.off, p.node, p.off); } catch (e) {}
  }
}

// For the self-test: the internals it drives directly.
window.tinyMdTest = {
  ready: function() { return useMarked; },
  editable: function() { return editable; },
  load: function(text) { lastPane = 'preview'; loadDocument(text, 'test.md'); },
  source: function() { return editor.value; },
  blocks: function() { return blocks; },
  blockMap: blockMap,
  pointAt: pointAt,
  srcToDom: srcToDom,
  setPane: function(p) { lastPane = p; },
  typeText: typeText, pressEnter: pressEnter, deleteDirection: deleteDirection,
  deleteSelection: deleteSelection, undo: undo, redo: redo, commands: commands,
  findNext: function(t) { lastFind = t; return findNext(); },
  dirty: function() { return editor.value !== savedValue; },
};
})();
