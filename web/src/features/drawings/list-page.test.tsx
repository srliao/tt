import { describe, expect, it } from 'vitest';
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
