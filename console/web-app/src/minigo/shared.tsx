import { useEffect, useState, type ReactNode, type FormEvent } from "react";
import { AlertCircle, Database, Loader2, X, CheckCircle2 } from "lucide-react";
import { api, type Data } from "./api";
import { Button } from "./components/ui/button";
import { Input } from "./components/ui/input";
import { Textarea } from "./components/ui/textarea";
import { Skeleton } from "./components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "./components/ui/dialog";
export { Button, Input, Textarea };
export function useData<T = Data>(
  path: string | null,
  query?: Data,
  revision = 0,
) {
  const [data, setData] = useState<T | null>(null),
    [error, setError] = useState(""),
    [errorCode, setErrorCode] = useState(""),
    [loading, setLoading] = useState(true);
  const key = JSON.stringify(query);
  useEffect(() => {
    let alive = true;
    setError("");
    setErrorCode("");
    setLoading(true);
    setData(null);
    if (!path) {
      setLoading(false);
      return;
    }
    api<T>(path, "GET", undefined, JSON.parse(key || "{}"))
      .then((d) => {
        if (alive) setData(d);
      })
      .catch((e) => {
        if (alive) {
          setError(e.message);
          setErrorCode(e.code || "");
        }
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [path, key, revision]);
  return { data, error, errorCode, loading };
}
export function Loading() {
  return (
    <div className="space-y-4 p-7" aria-label="Loading" role="status">
      <Skeleton className="h-5 w-1/3" />
      <Skeleton className="h-12 w-full" />
      <Skeleton className="h-12 w-full" />
      <Skeleton className="h-12 w-4/5" />
    </div>
  );
}
export function ErrorBox({ message }: { message: string }) {
  return message ? (
    <div className="error-box" role="alert">
      <AlertCircle size={18} />
      <span>{message}</span>
    </div>
  ) : null;
}
export function Empty({
  title = "Nothing here yet",
  children,
}: {
  title?: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Database size={24} />
      </div>
      <h3>{title}</h3>
      <p>{children || "This list will update when items become available."}</p>
    </div>
  );
}
export function Header({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        {eyebrow && <div className="eyebrow">{eyebrow}</div>}
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="heading-actions">{children}</div>
    </div>
  );
}
export function Notice({
  message,
  onClose,
}: {
  message: string;
  onClose: () => void;
}) {
  useEffect(() => {
    if (message) {
      const t = setTimeout(onClose, 6000);
      return () => clearTimeout(t);
    }
  }, [message]);
  return message ? (
    <div className="notice" role="status">
      <CheckCircle2 size={18} />
      <span>{message}</span>
      <button aria-label="Dismiss notification" onClick={onClose}>
        <X size={16} />
      </button>
    </div>
  ) : null;
}
export type Field = {
  name: string;
  label: string;
  type?: string;
  value?: string;
  required?: boolean;
  help?: string;
  placeholder?: string;
  options?: string[];
};
export type Editor = {
  title: string;
  description: string;
  fields: Field[];
  submit?: string;
  danger?: boolean;
  successMessage?: string | false;
  confirm?: string;
  run: (values: Record<string, string>) => Promise<unknown>;
  onSuccess?: () => void;
};
export function EditorDialog({
  editor,
  onClose,
  notify,
}: {
  editor: Editor | null;
  onClose: () => void;
  notify: (m: string) => void;
}) {
  const [values, setValues] = useState<Record<string, string>>({}),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    setValues(
      Object.fromEntries(
        (editor?.fields || []).map((f) => [f.name, f.value || ""]),
      ),
    );
    setError("");
  }, [editor]);
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!editor) return;
    setBusy(true);
    setError("");
    try {
      await editor.run(values);
      editor.onSuccess?.();
      if (editor.successMessage !== false)
        notify(editor.successMessage || `${editor.title} completed`);
      onClose();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open={!!editor}
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{editor?.title}</DialogTitle>
          <DialogDescription>{editor?.description}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-5">
          <ErrorBox message={error} />
          {editor?.fields.map((f) => (
            <div className="field" key={f.name}>
              <label htmlFor={`field-${f.name}`}>{f.label}</label>
              {f.type === "textarea" ? (
                <Textarea
                  id={`field-${f.name}`}
                  rows={10}
                  className="font-mono text-xs"
                  value={values[f.name] || ""}
                  required={f.required}
                  onChange={(e) =>
                    setValues({ ...values, [f.name]: e.target.value })
                  }
                />
              ) : f.options ? (
                <select
                  id={`field-${f.name}`}
                  value={values[f.name] || ""}
                  onChange={(e) =>
                    setValues({ ...values, [f.name]: e.target.value })
                  }
                >
                  {f.options.map((x) => (
                    <option key={x}>{x}</option>
                  ))}
                </select>
              ) : (
                <Input
                  id={`field-${f.name}`}
                  type={f.type || "text"}
                  autoComplete={f.type === "password" ? "new-password" : "off"}
                  value={values[f.name] || ""}
                  required={f.required}
                  placeholder={f.placeholder}
                  onChange={(e) =>
                    setValues({ ...values, [f.name]: e.target.value })
                  }
                />
              )}{" "}
              {f.help && <small>{f.help}</small>}
            </div>
          ))}
          {editor?.confirm && (
            <div className="field">
              <label htmlFor="confirmation">
                Type <strong>{editor.confirm}</strong> to confirm
              </label>
              <Input
                id="confirmation"
                value={values.__confirm || ""}
                autoComplete="off"
                onChange={(e) =>
                  setValues({ ...values, __confirm: e.target.value })
                }
              />
            </div>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant={editor?.danger ? "destructive" : "default"}
              disabled={
                busy ||
                !!(editor?.confirm && values.__confirm !== editor.confirm)
              }
            >
              {busy && <Loader2 className="animate-spin" size={16} />}{" "}
              {editor?.submit || "Save changes"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
export type Actions = {
  edit: (e: Editor) => void;
  notify: (m: string) => void;
  session: Data;
  navigate: (path: string) => void;
};
