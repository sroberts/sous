import { test } from 'node:test';
import assert from 'node:assert/strict';
import { inlineMarkdown } from '../src/lib/markdown.js';

test('Mela nutrition: bold labels, line breaks kept as text', () => {
  assert.deepEqual(inlineMarkdown('**Calories** 1063.75\n**Fat** 55.2 grams'), [
    { kind: 'strong', text: 'Calories' },
    { kind: 'text', text: ' 1063.75\n' },
    { kind: 'strong', text: 'Fat' },
    { kind: 'text', text: ' 55.2 grams' },
  ]);
});

test('italics and links', () => {
  assert.deepEqual(inlineMarkdown('See _this_ and [the source](https://example.com/a).'), [
    { kind: 'text', text: 'See ' },
    { kind: 'em', text: 'this' },
    { kind: 'text', text: ' and ' },
    { kind: 'link', text: 'the source', href: 'https://example.com/a' },
    { kind: 'text', text: '.' },
  ]);
});

test('unsafe links keep their text only', () => {
  assert.deepEqual(inlineMarkdown('[click](javascript:alert(1))'), [
    { kind: 'text', text: 'click' },
    { kind: 'text', text: ')' },
  ]);
});

test('plain text with stray symbols is left alone', () => {
  const s = '2 * 3 = 6, snake_case_name, <b>not html</b>';
  assert.deepEqual(inlineMarkdown(s), [{ kind: 'text', text: s }]);
});
