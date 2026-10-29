import { describe, expect, it } from 'vitest';
import { bodyWithScene } from '@/api/drawings';
import { drawingNameFromFile } from './list-page';

describe('drawingNameFromFile', () => {
  it.each([
    ['plan.excalidraw', 'plan'],
    ['Plan.EXCALIDRAW', 'Plan'],
    ['arch diagram.json', 'arch diagram'],
    ['flow.excalidraw.png', 'flow'],
    ['flow.excalidraw.svg', 'flow'],
    ['v1.2 notes.excalidraw', 'v1.2 notes'],
    ['.excalidraw', 'Imported drawing'],
  ])('%s → %s', (input, want) => {
    expect(drawingNameFromFile(input)).toBe(want);
  });
});

describe('bodyWithScene', () => {
  it('embeds the serialized scene verbatim alongside other fields', () => {
    const scene = '{\n  "type": "excalidraw",\n  "elements": []\n}';
    expect(JSON.parse(bodyWithScene({ name: 'a "quoted" name' }, scene))).toEqual({
      name: 'a "quoted" name',
      scene: { type: 'excalidraw', elements: [] },
    });
    expect(JSON.parse(bodyWithScene({}, scene))).toEqual({
      scene: { type: 'excalidraw', elements: [] },
    });
  });
});
