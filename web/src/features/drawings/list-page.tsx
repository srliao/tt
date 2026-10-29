/**
 * The /drawings page. Lists Excalidraw canvases, most recently edited first.
 * "New drawing" creates an untitled row and drops straight into the editor;
 * renaming happens there. "Import" uploads an existing .excalidraw file (or
 * a PNG/SVG exported with the scene embedded).
 */

import { Link, useNavigate } from '@tanstack/react-router';
import { formatDistanceToNow, parseISO } from 'date-fns';
import { PlusIcon, Trash2Icon, UploadIcon } from 'lucide-react';
import { useRef, useState } from 'react';
import { useCreateDrawing, useDeleteDrawing, useDrawings } from '@/api/drawings';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import type { DrawingSummary } from '@/types/drawing';

const IMPORT_ACCEPT =
  '.excalidraw,.json,.png,.svg,application/vnd.excalidraw+json,application/json,image/png,image/svg+xml';

/**
 * Derives a drawing name from an uploaded file name, dropping the
 * `.excalidraw` / `.json` / `.png` / `.svg` extensions Excalidraw exports use
 * (including stacked ones like `plan.excalidraw.png`). Exported for tests.
 */
export function drawingNameFromFile(fileName: string): string {
  const name = fileName.replace(/(\.(excalidraw|json|png|svg))+$/i, '').trim();
  return name || 'Imported drawing';
}

function useDrawingCreation() {
  const navigate = useNavigate();
  const create = useCreateDrawing();
  const [importError, setImportError] = useState<string | null>(null);
  const [isParsing, setIsParsing] = useState(false);

  const openEditor = (d: { id: number }) => {
    void navigate({ to: '/drawings/$id', params: { id: String(d.id) } });
  };

  const createBlank = () => create.mutate({ name: 'Untitled drawing' }, { onSuccess: openEditor });

  const importFile = async (file: File) => {
    setImportError(null);
    setIsParsing(true);
    let scene: string;
    try {
      // Loaded on demand so the list page doesn't pull in the Excalidraw
      // bundle. loadFromBlob validates the file and runs restore(), which
      // migrates scenes saved by older Excalidraw versions.
      let excalidraw: typeof import('@excalidraw/excalidraw');
      try {
        excalidraw = await import('@excalidraw/excalidraw');
      } catch {
        setImportError("Couldn't load the drawing editor. Check your connection and try again.");
        return;
      }
      try {
        const data = await excalidraw.loadFromBlob(file, null, null);
        scene = excalidraw.serializeAsJSON(data.elements, data.appState, data.files, 'local');
      } catch {
        setImportError(`"${file.name}" isn't an Excalidraw file.`);
        return;
      }
    } finally {
      setIsParsing(false);
    }
    create.mutate(
      { name: drawingNameFromFile(file.name), scene },
      {
        onSuccess: openEditor,
        onError: (err) => setImportError(`Import failed: ${err.message}`),
      },
    );
  };

  return { createBlank, importFile, importError, isPending: create.isPending || isParsing };
}

export function DrawingsListPage() {
  const { data: drawings = [], isLoading } = useDrawings();
  const creation = useDrawingCreation();

  const showEmpty = !isLoading && drawings.length === 0;
  return (
    <section className="mx-auto flex w-full max-w-4xl flex-col gap-4 px-4 py-4">
      <header className="flex items-center justify-between">
        <h1 className="font-heading text-xl font-medium">Drawings</h1>
        <div className="flex items-center gap-2">
          <ImportButton onFile={creation.importFile} disabled={creation.isPending} />
          <Button size="sm" onClick={creation.createBlank} disabled={creation.isPending}>
            <PlusIcon /> New drawing
          </Button>
        </div>
      </header>

      {creation.importError && (
        <p role="alert" className="text-sm text-destructive">
          {creation.importError}
        </p>
      )}

      {showEmpty ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-12 text-center">
          <h2 className="font-heading text-lg">No drawings yet</h2>
          <p className="max-w-md text-sm text-muted-foreground">
            Sketch diagrams, plans, and whiteboard notes with Excalidraw. Drawings are saved to the
            tt database automatically as you draw.
          </p>
          <Button onClick={creation.createBlank} disabled={creation.isPending}>
            <PlusIcon /> Create your first drawing
          </Button>
        </div>
      ) : (
        <table aria-label="Drawings" className="w-full text-sm">
          <thead className="text-xs text-muted-foreground">
            <tr className="border-b">
              <th className="px-2 py-2 text-left font-medium">Name</th>
              <th className="w-40 px-2 py-2 text-left font-medium">Last edited</th>
              <th className="w-8 px-1 py-2" />
            </tr>
          </thead>
          <tbody>
            {drawings.map((d) => (
              <DrawingRow key={d.id} drawing={d} />
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function ImportButton({ onFile, disabled }: { onFile: (file: File) => void; disabled: boolean }) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <Button
        size="sm"
        variant="outline"
        onClick={() => input.current?.click()}
        disabled={disabled}
      >
        <UploadIcon /> Import
      </Button>
      <input
        ref={input}
        type="file"
        accept={IMPORT_ACCEPT}
        className="hidden"
        data-testid="import-drawing-input"
        onChange={(e) => {
          const file = e.target.files?.[0];
          // Reset so picking the same file again still fires onChange.
          e.target.value = '';
          if (file) onFile(file);
        }}
      />
    </>
  );
}

function DrawingRow({ drawing }: { drawing: DrawingSummary }) {
  const deleteDrawing = useDeleteDrawing();
  const [confirmOpen, setConfirmOpen] = useState(false);

  return (
    <tr data-drawing-id={drawing.id} className="border-b hover:bg-muted/40">
      <td className="px-2 py-2 align-middle">
        <Link
          to="/drawings/$id"
          params={{ id: String(drawing.id) }}
          className="text-left text-sm font-medium hover:underline"
        >
          {drawing.name}
        </Link>
      </td>
      <td className="px-2 py-2 align-middle">
        <span className="text-xs text-muted-foreground">
          {formatDistanceToNow(parseISO(drawing.updated_at), { addSuffix: true })}
        </span>
      </td>
      <td className="px-1 py-2 align-middle">
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={`Delete ${drawing.name}`}
          onClick={() => setConfirmOpen(true)}
        >
          <Trash2Icon />
        </Button>
        <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete drawing "{drawing.name}"?</AlertDialogTitle>
              <AlertDialogDescription>This permanently removes the drawing.</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                onClick={() => {
                  deleteDrawing.mutate(drawing.id);
                  setConfirmOpen(false);
                }}
              >
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </td>
    </tr>
  );
}
