// Rendered-editing checks, run inside the real page by `tinymd.exe --selftest OUT`.
// Each case loads a document, puts the caret or selection on rendered text, makes
// an edit through the same code the keyboard uses, and compares the Markdown.
var selftestErrors = [];
window.addEventListener('error', function(e) {
  selftestErrors.push((e.message || 'error') + ' @' + (e.filename || '') + ':' + (e.lineno || '') + ' ' + (e.target && e.target.src || ''));
}, true);
window.addEventListener('DOMContentLoaded', function() {
  var results = [];
  function T() { return window.tinyMdTest; }

  function locate(text, offset) {
    var bs = T().blocks();
    for (var b = 0; b < bs.length; b++) {
      var t = T().blockMap(bs[b]).text, i = t.indexOf(text);
      if (i >= 0) return T().pointAt(b, i + (offset || 0));
    }
    throw new Error('"' + text + '" is not in the rendered text');
  }
  function place(text, offset) {
    var p = locate(text, offset);
    window.getSelection().setBaseAndExtent(p.node, p.off, p.node, p.off);
  }
  function select(text) {
    var a = locate(text, 0), c = locate(text, text.length);
    window.getSelection().setBaseAndExtent(a.node, a.off, c.node, c.off);
  }
  function check(name, fn) {
    try {
      var detail = fn();
      results.push({ name: name, ok: detail === true, detail: detail === true ? '' : String(detail) });
    } catch (e) {
      results.push({ name: name, ok: false, detail: String(e && e.stack || e) });
    }
  }
  function expect(want) {
    var got = T().source();
    return got === want ? true : 'got ' + JSON.stringify(got) + ' want ' + JSON.stringify(want);
  }
  function load(text) { T().load(text); }

  var sample = '# Title é\n\nSome **bold** text and a [link](http://example.com) here,\nwrapped onto a second line with `code`.\n\n' +
    '- first item\n- second *item*\n\n> quoted line one\n> quoted line two\n>\n> second quoted paragraph\n\n' +
    '| Name | Value |\n| :--- | ---: |\n| alpha | 1 |\n| beta 😀 | 2 |\n\n```go\nfunc main() {\n\tprintln("hi")\n}\n```\n\n---\n\n1. one\n2. two\n';

  function run() {
    check('the page became editable', function() {
      load(sample);
      return T().editable() || 'rendered pane is read-only';
    });

    check('every mapped character points at its own source', function() {
      load(sample);
      var src = T().source(), bs = T().blocks(), n = 0;
      for (var b = 0; b < bs.length; b++) {
        var m = T().blockMap(bs[b]);
        for (var i = 0; i < m.map.length; i++) {
          if (m.map[i] < 0) continue;
          if (src[m.map[i]] !== m.text[i]) return 'block ' + b + ' char ' + JSON.stringify(m.text[i]) + ' maps to ' + JSON.stringify(src[m.map[i]]);
          n++;
        }
      }
      return n > 150 || 'only ' + n + ' characters mapped';
    });

    check('typing inside bold extends the bold text', function() {
      load(sample);
      place('bold', 2);
      T().typeText('XY');
      T().typeText('Z');
      return expect(sample.replace('**bold**', '**boXYZld**'));
    });

    check('an edit leaves every other byte alone, and undo restores them', function() {
      load(sample);
      place('Title', 5);
      T().typeText('!');
      var r = expect(sample.replace('# Title é', '# Title! é'));
      if (r !== true) return r;
      T().undo();
      if (T().source() !== sample) return 'undo: ' + JSON.stringify(T().source());
      if (T().dirty()) return 'still marked modified after undoing to the saved text';
      T().redo();
      return expect(sample.replace('# Title é', '# Title! é'));
    });

    check('deleting the only bold character removes its markers', function() {
      load('a **b** c\n');
      place('b', 1);
      T().deleteDirection(true);
      return expect('a  c\n');
    });

    check('deleting across formatting keeps markers balanced', function() {
      load('Some **bold** text\n');
      select('ld te');
      T().deleteSelection();
      return expect('Some **bo**xt\n');
    });

    check('typing over a selection replaces it', function() {
      load(sample);
      select('first item');
      T().typeText('replaced');
      return expect(sample.replace('- first item', '- replaced'));
    });

    check('Enter in a list item starts a new item', function() {
      load('- first item\n- second\n');
      place('first item', 10);
      T().pressEnter();
      T().typeText('inserted');
      return expect('- first item\n- inserted\n- second\n');
    });

    check('Enter in an ordered list numbers the new item', function() {
      load('1. one\n2. two\n');
      place('one', 3);
      T().pressEnter();
      T().typeText('half');
      return expect('1. one\n2. half\n2. two\n');
    });

    check('Enter on an empty item leaves the list', function() {
      load('- one\n');
      place('one', 3);
      T().pressEnter();
      T().pressEnter();
      if (T().source() !== '- one\n') return 'after second Enter: ' + JSON.stringify(T().source());
      T().typeText('after');
      return expect('- one\n\nafter\n');
    });

    check('Enter at the end of a paragraph starts a new one', function() {
      load('First paragraph.\n\nSecond.\n');
      place('First paragraph.', 16);
      T().pressEnter();
      T().typeText('Middle');
      return expect('First paragraph.\n\nMiddle\n\nSecond.\n');
    });

    check('Enter in the middle of a paragraph splits it', function() {
      load('Firstsecond\n');
      place('Firstsecond', 5);
      T().pressEnter();
      T().typeText('S');
      return expect('First\n\nSsecond\n');
    });

    check('Backspace at the start of a block joins it to the one before', function() {
      load('# Heading\n\nParagraph\n');
      place('Paragraph', 0);
      T().deleteDirection(true);
      var r = expect('# HeadingParagraph\n');
      if (r !== true) return r;
      T().typeText(' ');
      return expect('# Heading Paragraph\n');
    });

    check('Backspace at the start of a list item joins it to the item before', function() {
      load('- one\n- two\n');
      place('two', 0);
      T().deleteDirection(true);
      return expect('- onetwo\n');
    });

    check('table cells are editable and pipes are escaped', function() {
      load('| Name | Value |\n| --- | --- |\n| alpha | 1 |\n');
      place('alpha', 5);
      T().typeText('|x');
      var r = expect('| Name | Value |\n| --- | --- |\n| alpha\\|x | 1 |\n');
      if (r !== true) return r;
      place('alpha', 5);
      T().pressEnter();
      return expect('| Name | Value |\n| --- | --- |\n| alpha\\|x | 1 |\n');
    });

    check('code block editing keeps the fence', function() {
      load('```\nline one\n```\n');
      place('line one', 8);
      T().pressEnter();
      T().typeText('line two');
      return expect('```\nline one\nline two\n```\n');
    });

    check('typing in a quote and a link', function() {
      load('> quoted text\n\nSee [the site](http://x.y) now.\n');
      place('quoted', 6);
      T().typeText('!');
      place('the site', 8);
      T().typeText(' here');
      return expect('> quoted! text\n\nSee [the site here](http://x.y) now.\n');
    });

    check('supplementary characters', function() {
      load('a😀b\n');
      place('b', 0);
      T().deleteDirection(true);
      var r = expect('ab\n');
      if (r !== true) return r;
      T().typeText('é😀');
      return expect('aé😀b\n');
    });

    check('bold button wraps the rendered selection and toggles back', function() {
      load('plain words here\n');
      T().setPane('preview');
      select('words');
      T().commands.bold();
      var r = expect('plain **words** here\n');
      if (r !== true) return r;
      if (window.getSelection().toString() !== 'words') return 'selection after bold: ' + window.getSelection().toString();
      T().commands.bold();
      return expect('plain words here\n');
    });

    check('italic on bold text makes bold italic', function() {
      load('a **b** c\n');
      select('b');
      T().commands.italic();
      return expect('a ***b*** c\n');
    });

    check('bold with no selection takes the word at the caret', function() {
      load('one two three\n');
      place('two', 1);
      T().commands.bold();
      return expect('one **two** three\n');
    });

    check('heading and list buttons work on the caret line', function() {
      load('Intro\n\nline\n');
      place('line', 2);
      T().commands.heading();
      var r = expect('Intro\n\n# line\n');
      if (r !== true) return r;
      T().commands.heading(); T().commands.heading(); T().commands.heading();
      r = expect('Intro\n\nline\n');
      if (r !== true) return 'heading did not cycle back: ' + r;
      T().commands.list();
      r = expect('Intro\n\n- line\n');
      if (r !== true) return r;
      T().commands.list();
      r = expect('Intro\n\nline\n');
      if (r !== true) return r;
      T().typeText('X');
      return expect('Intro\n\nliXne\n');
    });

    check('code button marks inline code', function() {
      load('call foo now\n');
      select('foo');
      T().commands.code();
      return expect('call `foo` now\n');
    });

    check('find selects the rendered match and wraps', function() {
      load('Alpha **beta** gamma\n\nBeta again\n');
      place('Alpha', 0);
      if (!T().findNext('beta') || window.getSelection().toString() !== 'beta') return 'first: ' + window.getSelection().toString();
      if (!T().findNext('beta') || window.getSelection().toString() !== 'Beta') return 'second: ' + window.getSelection().toString();
      return T().findNext('beta') && window.getSelection().toString() === 'beta' || 'did not wrap';
    });

    check('the keyboard path: beforeinput is turned into source edits', function() {
      load('Hello world\n');
      var preview = document.getElementById('preview');
      preview.focus();
      place('world', 0);
      var ok = document.execCommand('insertText', false, 'big ');
      if (!document.hasFocus() && T().source() === 'Hello world\n') return true; // hidden window: no keyboard focus to test with
      if (!ok) return 'execCommand refused';
      return expect('Hello big world\n');
    });

    check('typing in the source pane renders and is undoable', function() {
      load('abc\n');
      T().setPane('editor');
      var ed = document.getElementById('editor');
      ed.focus();
      ed.setSelectionRange(3, 3);
      ed.setRangeText('d', 3, 3, 'end');
      ed.dispatchEvent(new InputEvent('input', { inputType: 'insertText', data: 'd' }));
      if (document.getElementById('preview').textContent.indexOf('abcd') < 0) return 'preview did not follow';
      T().undo();
      return expect('abc\n');
    });
  }

  // Search from the keyboard: Ctrl+F opens the find box, Enter finds, F3 finds the
  // next match. These go through the real key handlers, which work asynchronously.
  function key(target, k, ctrl) {
    target.dispatchEvent(new KeyboardEvent('keydown', { key: k, ctrlKey: !!ctrl, bubbles: true, cancelable: true }));
  }
  function keyboardSearch(done) {
    var step = function(fn, next) { setTimeout(function() { try { fn(); } catch (e) { results.push({ name: 'keyboard search', ok: false, detail: String(e) }); } next(); }, 50); };
    load('alpha beta\n\nsecond beta\n');
    T().setPane('preview');
    place('alpha', 0);
    step(function() { key(document, 'f', true); }, function() {
      step(function() {
        var input = document.getElementById('askInput');
        var shown = document.getElementById('ask').classList.contains('show');
        results.push({ name: 'Ctrl+F opens the find box', ok: shown, detail: shown ? '' : 'find box not shown' });
        input.value = 'beta';
        key(input, 'Enter');
      }, function() {
        step(function() {
          var s = window.getSelection().toString(), ok1 = s === 'beta' && T().blocks()[0].el.contains(window.getSelection().anchorNode);
          results.push({ name: 'Enter in the find box selects the first match', ok: ok1, detail: ok1 ? '' : 'selection ' + JSON.stringify(s) });
          key(document, 'F3');
        }, function() {
          step(function() {
            var ok2 = window.getSelection().toString() === 'beta' && T().blocks()[1].el.contains(window.getSelection().anchorNode);
            results.push({ name: 'F3 finds the next match', ok: ok2, detail: ok2 ? '' : 'selection in wrong place' });
          }, done);
        });
      });
    });
  }

  (function wait(tries) {
    if (window.tinyMdTest && window.tinyMdTest.ready()) {
      try { run(); } catch (e) { results.push({ name: 'harness', ok: false, detail: String(e) }); }
      keyboardSearch(function() {
        var failed = results.filter(function(r) { return !r.ok; }).length;
        goSelftestDone(JSON.stringify({ passed: results.length - failed, failed: failed, errors: selftestErrors, results: results }, null, 1));
      });
      return;
    }
    if (tries > 300) {
      goSelftestDone(JSON.stringify({ error: 'marked.js never loaded', page: !!window.tinyMdTest,
        marked: typeof marked, errors: selftestErrors }));
      return;
    }
    setTimeout(function() { wait(tries + 1); }, 100);
  })(0);
});
