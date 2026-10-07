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

  // Clipboard shortcut checks: Ctrl+C / Ctrl+Insert, Shift+Insert, Ctrl+Shift+C,
  // and the source textarea's copy event. Stubs replace the native bridge functions
  // and are restored afterwards.
  function clipboardChecks(done) {
    var origWrite = window.goWriteClipboard;
    var origRead = window.goReadClipboard;
    var origCopyPath = window.goCopyPath;

    function restore() {
      window.goWriteClipboard = origWrite;
      window.goReadClipboard = origRead;
      window.goCopyPath = origCopyPath;
    }

    function runStep(name, fn, next) {
      try {
        fn(function(detail) {
          try {
            results.push({ name: name, ok: detail === true, detail: detail === true ? '' : String(detail) });
          } finally {
            restore();
            next();
          }
        });
      } catch (e) {
        try {
          results.push({ name: name, ok: false, detail: String(e && e.stack || e) });
        } finally {
          restore();
          next();
        }
      }
    }

    var tests = [
      function(next) {
        runStep('rendered pane Ctrl+C copies selection and prevents default', function(cb) {
          load('Some **bold** text\n');
          T().setPane('preview');
          var preview = document.getElementById('preview');
          preview.focus();
          select('bold');
          var written = [];
          window.goWriteClipboard = function(text) { written.push(text); return 'ok'; };
          var ev = new KeyboardEvent('keydown', { key: 'c', ctrlKey: true, bubbles: true, cancelable: true });
          preview.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (!ev.defaultPrevented) {
                return cb('event default was not prevented' + (written.length ? ', stub received: ' + JSON.stringify(written) : ''));
              }
              if (written.length !== 1 || written[0] !== 'bold') {
                return cb('stub received ' + JSON.stringify(written) + ' want ["bold"]');
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('rendered pane Ctrl+Insert copies selection and prevents default', function(cb) {
          load('Some **bold** text\n');
          T().setPane('preview');
          var preview = document.getElementById('preview');
          preview.focus();
          select('bold');
          var written = [];
          window.goWriteClipboard = function(text) { written.push(text); return 'ok'; };
          var ev = new KeyboardEvent('keydown', { key: 'Insert', ctrlKey: true, bubbles: true, cancelable: true });
          preview.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (!ev.defaultPrevented) {
                return cb('event default was not prevented' + (written.length ? ', stub received: ' + JSON.stringify(written) : ''));
              }
              if (written.length !== 1 || written[0] !== 'bold') {
                return cb('stub received ' + JSON.stringify(written) + ' want ["bold"]');
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('markdown pane Ctrl+C copies selected substring', function(cb) {
          load('alpha beta gamma\n');
          T().setPane('editor');
          var ed = document.getElementById('editor');
          ed.focus();
          ed.setSelectionRange(6, 10);
          var written = [];
          window.goWriteClipboard = function(text) { written.push(text); return 'ok'; };
          var ev = new KeyboardEvent('keydown', { key: 'c', ctrlKey: true, bubbles: true, cancelable: true });
          ed.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (written.length !== 1 || written[0] !== 'beta') {
                return cb('stub received ' + JSON.stringify(written) + ' want ["beta"]' + (ev.defaultPrevented ? '' : ' (default not prevented)'));
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('rendered pane Ctrl+C with collapsed caret leaves event alone', function(cb) {
          load('Some **bold** text\n');
          T().setPane('preview');
          var preview = document.getElementById('preview');
          preview.focus();
          place('bold', 0);
          var written = [];
          window.goWriteClipboard = function(text) { written.push(text); return 'ok'; };
          var ev = new KeyboardEvent('keydown', { key: 'c', ctrlKey: true, bubbles: true, cancelable: true });
          preview.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (ev.defaultPrevented) {
                return cb('event was default-prevented with collapsed caret');
              }
              if (written.length !== 0) {
                return cb('stub was called unexpectedly with: ' + JSON.stringify(written));
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('rendered pane Shift+Insert replaces selection with clipboard text', function(cb) {
          load('Some **bold** text\n');
          T().setPane('preview');
          var preview = document.getElementById('preview');
          preview.focus();
          select('text');
          window.goReadClipboard = function() { return { text: 'ZED' }; };
          var ev = new KeyboardEvent('keydown', { key: 'Insert', shiftKey: true, bubbles: true, cancelable: true });
          preview.dispatchEvent(ev);
          setTimeout(function() {
            try {
              var src = T().source();
              var want = 'Some **bold** ZED\n';
              if (src !== want) {
                return cb('source got ' + JSON.stringify(src) + ' want ' + JSON.stringify(want) + (ev.defaultPrevented ? '' : ' (default not prevented)'));
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('Ctrl+Shift+C calls goCopyPath and not goWriteClipboard', function(cb) {
          load('Some **bold** text\n');
          T().setPane('preview');
          var preview = document.getElementById('preview');
          preview.focus();
          select('bold');
          var copyPathCalled = 0;
          var written = [];
          window.goCopyPath = function() { copyPathCalled++; return 'Path copied'; };
          window.goWriteClipboard = function(text) { written.push(text); return 'ok'; };
          var ev = new KeyboardEvent('keydown', { key: 'C', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true });
          preview.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (copyPathCalled === 0) {
                return cb('goCopyPath was not called' + (written.length ? ', goWriteClipboard called with: ' + JSON.stringify(written) : ''));
              }
              if (written.length !== 0) {
                return cb('goWriteClipboard was unexpectedly called with: ' + JSON.stringify(written));
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('copy event on textarea writes selected text to clipboard', function(cb) {
          load('alpha beta gamma\n');
          T().setPane('editor');
          var ed = document.getElementById('editor');
          ed.focus();
          ed.setSelectionRange(6, 10);
          var written = [];
          window.goWriteClipboard = function(text) { written.push(text); return 'ok'; };
          var dt = new DataTransfer();
          var ev = new ClipboardEvent('copy', {
            bubbles: true,
            cancelable: true,
            clipboardData: dt
          });
          ed.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (written.length !== 1 || written[0] !== 'beta') {
                return cb('stub received ' + JSON.stringify(written) + ' want ["beta"]');
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      }
    ];

    var idx = 0;
    function runNext() {
      if (idx >= tests.length) {
        try {
          restore();
        } finally {
          done();
        }
        return;
      }
      var test = tests[idx++];
      test(runNext);
    }
    runNext();
  }

  // Link Ctrl+click and navigation checks. Stubs replace the native bridge functions
  // and are restored afterwards.
  function linkChecks(done) {
    var origGoOpenLink = window.goOpenLink;
    var origOpen = window.open;

    function restore() {
      window.goOpenLink = origGoOpenLink;
      window.open = origOpen;
      document.body.classList.remove('ctrl-down');
    }

    function runStep(name, fn, next) {
      try {
        fn(function(detail) {
          try {
            results.push({ name: name, ok: detail === true, detail: detail === true ? '' : String(detail) });
          } finally {
            restore();
            next();
          }
        });
      } catch (e) {
        try {
          results.push({ name: name, ok: false, detail: String(e && e.stack || e) });
        } finally {
          restore();
          next();
        }
      }
    }

    var doc = '# Title\n\nSee [site](https://example.com/a) and [doc](other.md) and [top](#title).\n';

    function findLink(preview, href) {
      var a = preview.querySelector('a[href="' + href + '"]');
      if (a) return a;
      var all = preview.querySelectorAll('a');
      for (var i = 0; i < all.length; i++) {
        if (all[i].getAttribute('href') === href) return all[i];
      }
      return null;
    }

    var tests = [
      function(next) {
        runStep('Ctrl+click on the site link opens URL and prevents default', function(cb) {
          load(doc);
          T().setPane('preview');
          var preview = document.getElementById('preview');
          var a = findLink(preview, 'https://example.com/a');
          if (!a) return cb('link https://example.com/a not found in #preview');
          var opened = [];
          window.goOpenLink = function(href) { opened.push(href); return 'opened'; };
          var ev = new MouseEvent('click', { ctrlKey: true, bubbles: true, cancelable: true });
          a.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (!ev.defaultPrevented) {
                return cb('event default was not prevented' + (opened.length ? ', stub received: ' + JSON.stringify(opened) : ''));
              }
              if (opened.length !== 1 || opened[0] !== 'https://example.com/a') {
                return cb('stub received ' + JSON.stringify(opened) + ' want ["https://example.com/a"]');
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('plain click on the site link does not open and prevents default', function(cb) {
          load(doc);
          T().setPane('preview');
          var preview = document.getElementById('preview');
          var a = findLink(preview, 'https://example.com/a');
          if (!a) return cb('link https://example.com/a not found in #preview');
          var opened = [];
          window.goOpenLink = function(href) { opened.push(href); return 'opened'; };
          var ev = new MouseEvent('click', { ctrlKey: false, bubbles: true, cancelable: true });
          a.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (!ev.defaultPrevented) {
                return cb('event default was not prevented' + (opened.length ? ', stub received: ' + JSON.stringify(opened) : ''));
              }
              if (opened.length !== 0) {
                return cb('stub was called unexpectedly with: ' + JSON.stringify(opened));
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('Ctrl+click on the #title link does not call goOpenLink', function(cb) {
          load(doc);
          T().setPane('preview');
          var preview = document.getElementById('preview');
          var a = findLink(preview, '#title');
          if (!a) return cb('link #title not found in #preview');
          var opened = [];
          window.goOpenLink = function(href) { opened.push(href); return 'opened'; };
          var ev = new MouseEvent('click', { ctrlKey: true, bubbles: true, cancelable: true });
          a.dispatchEvent(ev);
          setTimeout(function() {
            try {
              if (opened.length !== 0) {
                return cb('stub was called unexpectedly for #title with: ' + JSON.stringify(opened));
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('every preview link has a title starting with Ctrl+click to open', function(cb) {
          load(doc);
          T().setPane('preview');
          setTimeout(function() {
            try {
              var preview = document.getElementById('preview');
              var links = preview.querySelectorAll('a[href]');
              if (!links || links.length === 0) {
                return cb('no a[href] links found in #preview');
              }
              for (var i = 0; i < links.length; i++) {
                var a = links[i];
                var title = a.getAttribute('title') || a.title || '';
                var prefix = 'Ctrl+click to open';
                if (title.indexOf(prefix) !== 0) {
                  return cb('link ' + i + ' (' + (a.getAttribute('href') || '') + ') title was ' + JSON.stringify(title) + ' want prefix ' + JSON.stringify(prefix));
                }
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('window.open returns null', function(cb) {
          setTimeout(function() {
            try {
              var res = window.open('https://example.com');
              if (res !== null) {
                return cb('window.open returned ' + JSON.stringify(res) + ' want null');
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('Control keydown adds ctrl-down to body and keyup removes it', function(cb) {
          document.body.classList.remove('ctrl-down');
          var kd = new KeyboardEvent('keydown', { key: 'Control', ctrlKey: true, bubbles: true, cancelable: true });
          document.dispatchEvent(kd);
          setTimeout(function() {
            try {
              if (!document.body.classList.contains('ctrl-down')) {
                return cb('body does not have ctrl-down class after Control keydown');
              }
              var ku = new KeyboardEvent('keyup', { key: 'Control', ctrlKey: false, bubbles: true, cancelable: true });
              document.dispatchEvent(ku);
              setTimeout(function() {
                try {
                  if (document.body.classList.contains('ctrl-down')) {
                    return cb('body still has ctrl-down class after Control keyup');
                  }
                  cb(true);
                } catch (e) { cb(e); }
              }, 50);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      },
      function(next) {
        runStep('plain click leaves document source unchanged', function(cb) {
          load(doc);
          T().setPane('preview');
          var preview = document.getElementById('preview');
          var a = findLink(preview, 'https://example.com/a');
          if (!a) return cb('link https://example.com/a not found in #preview');
          var before = T().source();
          var ev = new MouseEvent('click', { ctrlKey: false, bubbles: true, cancelable: true });
          a.dispatchEvent(ev);
          setTimeout(function() {
            try {
              var after = T().source();
              if (after !== before) {
                return cb('source changed: got ' + JSON.stringify(after) + ' want ' + JSON.stringify(before));
              }
              cb(true);
            } catch (e) { cb(e); }
          }, 50);
        }, next);
      }
    ];

    var idx = 0;
    function runNext() {
      if (idx >= tests.length) {
        try {
          restore();
        } finally {
          done();
        }
        return;
      }
      var test = tests[idx++];
      test(runNext);
    }
    runNext();
  }

  // Source highlighting and scroll sync checks.
  function syncChecks(done) {
    var ed = document.getElementById('editor');
    var preview = document.getElementById('preview');

    function delay(ms) {
      return new Promise(function(resolve) { setTimeout(resolve, ms); });
    }

    async function shot(name) {
      if (typeof window.goShot !== 'function') return;
      try {
        var res = await window.goShot(name);
        if (res !== 'ok') {
          results.push({ name: 'shot ' + name, ok: false, detail: 'got ' + JSON.stringify(res) + ' want "ok"' });
        }
      } catch (e) {
        results.push({ name: 'shot ' + name, ok: false, detail: String(e && e.stack || e) });
      }
    }

    function isMarkVisible() {
      var b = T().markBox();
      return !!(b && b.top >= ed.scrollTop - 1 && b.top + b.height <= ed.scrollTop + ed.clientHeight + 1);
    }

    var tests = [
      {
        name: 'sync: heading',
        shot: 'heading',
        fn: async function() {
          load(sample);
          preview.focus();
          select('Title');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          if (h.kind !== 'range') return 'kind got ' + JSON.stringify(h.kind) + ' want "range"';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'Title') return 'slice got ' + JSON.stringify(slice) + ' want "Title"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: emphasis',
        shot: 'emphasis',
        fn: async function() {
          load(sample);
          preview.focus();
          select('bold');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'bold') return 'slice got ' + JSON.stringify(slice) + ' want "bold"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: link text',
        shot: 'link-text',
        fn: async function() {
          load(sample);
          preview.focus();
          select('link');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'link') return 'slice got ' + JSON.stringify(slice) + ' want "link"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: inline code',
        shot: 'inline-code',
        fn: async function() {
          load(sample);
          preview.focus();
          select('code');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'code') return 'slice got ' + JSON.stringify(slice) + ' want "code"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: list item',
        shot: 'list-item',
        fn: async function() {
          load(sample);
          preview.focus();
          var bs = T().blocks();
          var listIndex = -1;
          for (var b = 0; b < bs.length; b++) {
            if (bs[b].tok && bs[b].tok.type === 'list') {
              listIndex = b;
              break;
            }
          }
          if (listIndex < 0) return 'list block not found in sample';
          var m = T().blockMap(bs[listIndex]);
          var i = m.text.indexOf('second');
          if (i < 0) return '"second" not found in list block map text';
          var a = T().pointAt(listIndex, i);
          var c = T().pointAt(listIndex, i + 6);
          window.getSelection().setBaseAndExtent(a.node, a.off, c.node, c.off);
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'second') return 'slice got ' + JSON.stringify(slice) + ' want "second"';
          if (h.s < sample.indexOf('- second *item*')) return 'highlight offset (' + h.s + ') matched earlier occurrence';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: table cell',
        shot: 'table-cell',
        fn: async function() {
          load(sample);
          preview.focus();
          select('alpha');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'alpha') return 'slice got ' + JSON.stringify(slice) + ' want "alpha"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: code block',
        shot: 'code-block',
        fn: async function() {
          load('```go\nfunc main() {\n\tprintln("hi")\n}\n```\n');
          preview.focus();
          select('println');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'println') return 'slice got ' + JSON.stringify(slice) + ' want "println"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: quote',
        shot: 'quote',
        fn: async function() {
          load(sample);
          preview.focus();
          select('quoted line two');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== 'quoted line two') return 'slice got ' + JSON.stringify(slice) + ' want "quoted line two"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: mid-element start/end',
        shot: 'mid-element',
        fn: async function() {
          load(sample);
          preview.focus();
          select('ld text and a li');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice.indexOf('ld') !== 0) return 'slice does not start with "ld": got ' + JSON.stringify(slice);
          if (slice.slice(-2) !== 'li') return 'slice does not end with "li": got ' + JSON.stringify(slice);
          if (slice.indexOf('** text and a [') < 0) return 'slice does not contain "** text and a [": got ' + JSON.stringify(slice);
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: cross-block',
        shot: 'cross-block',
        fn: async function() {
          load(sample);
          preview.focus();
          var a = locate('wrapped', 0);
          var c = locate('first item', 'first item'.length);
          window.getSelection().setBaseAndExtent(a.node, a.off, c.node, c.off);
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          var slice = T().source().slice(h.s, h.e);
          if (slice.indexOf('wrapped') !== 0) return 'slice does not start with "wrapped": got ' + JSON.stringify(slice);
          if (slice.slice(-10) !== 'first item') return 'slice does not end with "first item": got ' + JSON.stringify(slice);
          if (slice.indexOf('\n\n- ') < 0) return 'slice does not contain "\\n\\n- ": got ' + JSON.stringify(slice);
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: caret at line start',
        shot: 'caret-start',
        fn: async function() {
          load(sample);
          preview.focus();
          place('first item', 0);
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          if (h.kind !== 'caret') return 'kind got ' + JSON.stringify(h.kind) + ' want "caret"';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== '- first item') return 'slice got ' + JSON.stringify(slice) + ' want "- first item"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: caret at line end',
        shot: 'caret-end',
        fn: async function() {
          load(sample);
          preview.focus();
          place('quoted line one', 15);
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          if (h.kind !== 'caret') return 'kind got ' + JSON.stringify(h.kind) + ' want "caret"';
          var slice = T().source().slice(h.s, h.e);
          if (slice !== '> quoted line one') return 'slice got ' + JSON.stringify(slice) + ' want "> quoted line one"';
          if (!isMarkVisible()) return 'mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          return true;
        }
      },
      {
        name: 'sync: long document',
        shot: 'long-document',
        fn: async function() {
          var lines = [];
          for (var i = 1; i <= 300; i++) lines.push('Paragraph ' + i + '.');
          var longDoc = lines.join('\n\n') + '\n';
          load(longDoc);
          preview.focus();
          select('Paragraph 280.');
          await delay(150);
          var h1 = T().highlight();
          if (!h1) return 'Paragraph 280 highlight is null';
          var s1 = T().source().slice(h1.s, h1.e);
          if (s1 !== 'Paragraph 280.') return 'Paragraph 280 slice got ' + JSON.stringify(s1) + ' want "Paragraph 280."';
          if (!isMarkVisible()) return 'Paragraph 280 mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          var st1 = ed.scrollTop;
          if (st1 <= 0) return 'Paragraph 280 expected ed.scrollTop > 0, got ' + st1;
          // The rendered pane is scrolled to the selection too, so the shot shows both.
          locate('Paragraph 280.', 0).node.parentNode.scrollIntoView({ block: 'center' });
          await shot('long-document-scrolled-down');

          select('Paragraph 3.');
          await delay(150);
          var h2 = T().highlight();
          if (!h2) return 'Paragraph 3 highlight is null';
          var s2 = T().source().slice(h2.s, h2.e);
          if (s2 !== 'Paragraph 3.') return 'Paragraph 3 slice got ' + JSON.stringify(s2) + ' want "Paragraph 3."';
          if (!isMarkVisible()) return 'Paragraph 3 mark not visible: box=' + JSON.stringify(T().markBox()) + ' scrollTop=' + ed.scrollTop + ' clientHeight=' + ed.clientHeight;
          var st2 = ed.scrollTop;
          if (st2 >= st1) return 'Paragraph 3 expected ed.scrollTop < ' + st1 + ', got ' + st2;
          return true;
        }
      },
      {
        name: 'sync: no fighting the user',
        shot: 'no-fighting',
        fn: async function() {
          var lines = [];
          for (var i = 1; i <= 300; i++) lines.push('Paragraph ' + i + '.');
          var longDoc = lines.join('\n\n') + '\n';
          load(longDoc);
          preview.focus();
          select('Paragraph 150.');
          await delay(150);
          var stBefore = ed.scrollTop;
          select('Paragraph 151.');
          await delay(150);
          var stAfter = ed.scrollTop;
          if (stAfter !== stBefore) return 'ed.scrollTop changed from ' + stBefore + ' to ' + stAfter;
          var h = T().highlight();
          if (!h) return 'Paragraph 151 highlight is null';
          var s = T().source().slice(h.s, h.e);
          if (s !== 'Paragraph 151.') return 'Paragraph 151 slice got ' + JSON.stringify(s) + ' want "Paragraph 151."';
          if (!isMarkVisible()) return 'Paragraph 151 mark not visible';
          return true;
        }
      },
      {
        name: 'sync: no focus or selection stealing',
        shot: 'no-stealing',
        fn: async function() {
          load(sample);
          ed.focus();
          ed.setSelectionRange(5, 12);
          var startBefore = ed.selectionStart;
          var endBefore = ed.selectionEnd;
          preview.focus();
          select('bold');
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null';
          if (document.activeElement !== preview) return 'activeElement is ' + (document.activeElement ? document.activeElement.id || document.activeElement.tagName : 'null') + ' want #preview';
          if (ed.selectionStart !== startBefore || ed.selectionEnd !== endBefore) {
            return 'editor selection changed: got ' + ed.selectionStart + '-' + ed.selectionEnd + ' want ' + startBefore + '-' + endBefore;
          }
          return true;
        }
      },
      {
        name: 'sync: typing still works',
        shot: 'typing',
        fn: async function() {
          load(sample);
          preview.focus();
          place('bold', 2);
          T().typeText('XY');
          var src = T().source();
          if (src.indexOf('**boXYld**') < 0) return 'source missing "**boXYld**": got ' + JSON.stringify(src);
          await delay(150);
          var h = T().highlight();
          if (!h) return 'highlight is null after typing';
          if (h.kind !== 'caret') return 'highlight kind got ' + JSON.stringify(h.kind) + ' want "caret"';
          var slice = src.slice(h.s, h.e);
          if (slice.indexOf('boXYld') < 0) return 'highlight line got ' + JSON.stringify(slice) + ' want it to contain "boXYld"';
          return true;
        }
      },
      {
        name: 'sync: edit after mapping',
        shot: 'edit-after-mapping',
        fn: async function() {
          load('aaa\n\nbbb target\n');
          preview.focus();
          select('target');
          await delay(150);
          var h1 = T().highlight();
          var src1 = T().source();
          if (!h1 || src1.slice(h1.s, h1.e) !== 'target') return 'initial target highlight failed: ' + (h1 ? JSON.stringify(src1.slice(h1.s, h1.e)) : 'null');
          place('aaa', 0);
          T().typeText('PREFIX ');
          select('target');
          await delay(150);
          var h2 = T().highlight();
          var src2 = T().source();
          if (!h2 || src2.slice(h2.s, h2.e) !== 'target') return 'post-edit target highlight failed: ' + (h2 ? JSON.stringify(src2.slice(h2.s, h2.e)) : 'null');
          return true;
        }
      },
      {
        name: 'sync: editor focus clears',
        fn: async function() {
          load(sample);
          preview.focus();
          select('bold');
          await delay(150);
          if (!T().highlight()) return 'highlight not set before ed.focus()';
          ed.focus();
          await delay(150);
          var h = T().highlight();
          if (h !== null) return 'expected null highlight after ed.focus(), got ' + JSON.stringify(h);
          return true;
        }
      },
      {
        name: 'sync: rendered-only view',
        fn: async function() {
          load(sample);
          T().commands.viewRendered();
          preview.focus();
          select('bold');
          await delay(150);
          var h = T().highlight();
          var src = T().source();
          var ok = h && src.slice(h.s, h.e) === 'bold';
          T().commands.split();
          if (!ok) return 'rendered-only view highlight failed: got ' + (h ? JSON.stringify(src.slice(h.s, h.e)) : 'null');
          return true;
        }
      }
    ];

    var idx = 0;
    async function runNext() {
      if (idx >= tests.length) {
        done();
        return;
      }
      var t = tests[idx++];
      try {
        T().commands.split();
        var detail = await t.fn();
        results.push({ name: t.name, ok: detail === true, detail: detail === true ? '' : String(detail) });
        if (t.shot) await shot(t.shot);
      } catch (e) {
        results.push({ name: t.name, ok: false, detail: String(e && e.stack || e) });
        if (t.shot) {
          try { await shot(t.shot); } catch (_) {}
        }
      }
      runNext();
    }
    runNext();
  }

  (function wait(tries) {
    if (window.tinyMdTest && window.tinyMdTest.ready()) {
      try { run(); } catch (e) { results.push({ name: 'harness', ok: false, detail: String(e) }); }
      keyboardSearch(function() {
        clipboardChecks(function() {
          linkChecks(function() {
            syncChecks(function() {
              var failed = results.filter(function(r) { return !r.ok; }).length;
              goSelftestDone(JSON.stringify({ passed: results.length - failed, failed: failed, errors: selftestErrors, results: results }, null, 1));
            });
          });
        });
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
