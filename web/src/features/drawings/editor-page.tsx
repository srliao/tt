/**
 * The /drawings/:id page: a full-bleed Excalidraw canvas backed by the
 * drawings table.
 *
 * Persistence lives in `useSceneAutosave` (debounce, skip-identical,
 * baseline-until-interaction, one save in flight, flush on unmount/close).
 *
 * The canvas wrapper carries `data-app-shortcuts="off"` so the global
 * shortcut handler and command palette leave Excalidraw's single-letter
 * tool keys alone (see `isShortcutOptOut` in `lib/shortcuts.ts`).
 *
 * The canvas theme is deliberately NOT the app theme — see `canvas-theme.ts`.
 */

import { Excalidraw, serializeAsJSON } from '@excalidraw/excalidraw';
import '@excalidraw/excalidraw/index.css';
import type { ExcalidrawInitialDataState } from '@excalidraw/excalidraw/types';
import { Link } from '@tanstack/react-router';
import { ArrowLeftIcon } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useDrawing, useRenameDrawing, useSaveDrawingScene } from '@/api/drawings';
import { Input } from '@/components/ui/input';
import { useCanvasTheme } from './canvas-theme';
import { useSceneAutosave } from './use-scene-autosave';

// Fonts are self-hosted under dist/assets/excalidraw/ by the
// `excalidrawFonts` plugin in vite.config.ts. Must be set before the first
// render, which is when Excalidraw starts loading fonts.
declare global {
  interface Window {
    EXCALIDRAW_ASSET_PATH?: string | string[];
  }
}
window.EXCALIDRAW_ASSET_PATH = '/assets/excalidraw/';

type ChangeArgs = Parameters<NonNullable<React.ComponentProps<typeof Excalidraw>['onChange']>>;

export function DrawingEditorPage({ id }: { id: number }) {
  const { data: drawing, isLoading, error } = useDrawing(id);

  if (isLoading) return <div className="p-6 text-sm text-muted-foreground">Loading…</div>;
  if (error || !drawing) return <div className="p-6">Drawing not found.</div>;

  return (
    <DrawingCanvas
      id={id}
      name={drawing.name}
      scene={drawing.scene as ExcalidrawInitialDataState}
    />
  );
}

function DrawingCanvas({
  id,
  name,
  scene,
}: {
  id: number;
  name: string;
  scene: ExcalidrawInitialDataState;
}) {
  const [canvasTheme, setCanvasTheme] = useCanvasTheme();

  // Seed once; the canvas owns the scene after mount.
  const [initialData] = useState<ExcalidrawInitialDataState>(() => ({
    ...scene,
    scrollToContent: true,
  }));

  const { mutateAsync: saveScene } = useSaveDrawingScene();
  const autosave = useSceneAutosave((scene) => saveScene({ id, scene }));
  const { notifyChange } = autosave;

  const onChange = useCallback(
    (...[elements, appState, files]: ChangeArgs) => {
      // Excalidraw's menu toggle / Shift+Alt+D flips appState.theme; mirror it
      // so the choice sticks. Unchanged values are ignored.
      if (appState.theme === 'light' || appState.theme === 'dark') setCanvasTheme(appState.theme);
      notifyChange(() => serializeAsJSON(elements, appState, files, 'local'));
    },
    [notifyChange, setCanvasTheme],
  );

  return (
    <div className="fixed inset-x-0 top-14 bottom-0 flex flex-col">
      <div className="flex h-11 shrink-0 items-center gap-2 border-b px-3">
        <Link
          to="/drawings"
          aria-label="Back to drawings"
          className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <ArrowLeftIcon className="size-4" />
        </Link>
        <NameField id={id} name={name} />
        <span className="ml-auto text-xs text-muted-foreground" data-testid="save-status">
          {autosave.status === 'saved' && 'Saved'}
          {autosave.status === 'pending' && 'Unsaved changes'}
          {autosave.status === 'saving' && 'Saving…'}
          {autosave.status === 'error' && (
            <span className="text-destructive">Save failed — retrying on next edit</span>
          )}
        </span>
      </div>
      <div
        className="min-h-0 flex-1"
        data-app-shortcuts="off"
        onPointerDownCapture={autosave.markInteracted}
        onKeyDownCapture={autosave.markInteracted}
        onPasteCapture={autosave.markInteracted}
        onDropCapture={autosave.markInteracted}
      >
        <Excalidraw
          initialData={initialData}
          onChange={onChange}
          theme={canvasTheme}
          name={name}
          UIOptions={{ canvasActions: { loadScene: false, toggleTheme: true } }}
        />
      </div>
    </div>
  );
}

function NameField({ id, name }: { id: number; name: string }) {
  const rename = useRenameDrawing();
  const [value, setValue] = useState(name);

  const commit = () => {
    const next = value.trim();
    if (!next) {
      setValue(name);
      return;
    }
    if (next !== name) rename.mutate({ id, name: next });
  };

  return (
    <Input
      aria-label="Drawing name"
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') e.currentTarget.blur();
        if (e.key === 'Escape') {
          setValue(name);
          e.currentTarget.blur();
        }
      }}
      className="h-8 max-w-xs border-transparent bg-transparent shadow-none hover:border-input focus-visible:border-input"
    />
  );
}
