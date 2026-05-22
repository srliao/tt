import { act, fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CheatsheetApi, parseCheatsheet } from './cheatsheet-api';

describe('parseCheatsheet', () => {
  it('splits H2 headings into entries with descriptions', () => {
    const md = `# Title\n\n## ctx.today()\nReturns the date.\n\n## ctx.year()\nReturns the year.\n`;
    expect(parseCheatsheet(md)).toEqual([
      { name: 'ctx.today()', description: 'Returns the date.' },
      { name: 'ctx.year()', description: 'Returns the year.' },
    ]);
  });

  it('skips the top-level # heading', () => {
    expect(parseCheatsheet('# top\n## ctx.now()\nNow.\n')).toEqual([
      { name: 'ctx.now()', description: 'Now.' },
    ]);
  });
});

describe('CheatsheetApi', () => {
  it('lists ctx entries and copies the function name on click', async () => {
    const writeText = vi.fn().mockResolvedValueOnce(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });

    render(<CheatsheetApi />);
    // ctx.today() comes straight from the bundled cheatsheet markdown.
    const copyButton = screen.getByRole('button', { name: /Copy ctx\.today\(\)/i });
    await act(async () => {
      fireEvent.click(copyButton);
    });
    expect(writeText).toHaveBeenCalledWith('ctx.today()');
  });
});
