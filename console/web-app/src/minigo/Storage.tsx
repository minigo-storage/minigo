import { useEffect, useRef, useState } from "react";
import {
  ArrowDownToLine,
  ArrowRight,
  ArrowUpToLine,
  ChevronRight,
  Database,
  File,
  Folder,
  HardDrive,
  Layers,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  ShieldCheck,
  Trash2,
  Clock3,
  Link2,
} from "lucide-react";
import { api, bytes, can, date, enc, type Data, url } from "./api";
import {
  Button,
  Input,
  Empty,
  ErrorBox,
  Header,
  Loading,
  useData,
  type Actions,
} from "./shared";
import { Card, CardContent } from "./components/ui/card";
import { Badge } from "./components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "./components/ui/dialog";
export function Overview(a: Actions) {
  const { data, error, loading } = useData("/minigo/server"),
    buckets = useData("/buckets");
  const servers: Data[] = data?.servers || [];
  return (
    <>
      <Header
        eyebrow="YOUR WORKSPACE"
        title="Storage overview"
        description="Your objects, infrastructure, and access. All in one place."
      >
        <Button onClick={() => a.navigate("/buckets")}>
          Browse buckets <ArrowRight size={16} />
        </Button>
      </Header>
      <div className="overview-banner">
        <div>
          <div className="banner-icon">
            <Layers size={22} />
          </div>
          <h2>A home for every object.</h2>
          <p>S3-compatible storage. Familiar tools. Complete control.</p>
        </div>
        <div className="banner-grid" aria-hidden="true">
          <div />
          <div />
          <div />
          <div />
          <div />
          <div />
          <div />
          <div />
          <div />
        </div>
        <span className="banner-label">MINIGO / STORAGE</span>
      </div>
      <div className="stats">
        {[
          {
            label: "Buckets",
            value: data?.buckets ?? buckets.data?.buckets?.length,
            icon: Database,
            sub: "Accessible storage spaces",
          },
          {
            label: "Objects",
            value: data?.objects,
            icon: Layers,
            sub: "Across this deployment",
          },
          {
            label: "Storage used",
            value: data ? bytes(data.usageBytes) : undefined,
            icon: HardDrive,
            sub: "Reported by storage scanner",
          },
          {
            label: "Servers",
            value: data?.servers?.length,
            icon: ShieldCheck,
            sub: data?.mode || "Deployment status",
          },
        ].map((s) => (
          <Card key={s.label}>
            <CardContent className="stat">
              <div>
                <span>{s.label}</span>
                <s.icon size={18} />
              </div>
              <strong>
                {loading && !buckets.data
                  ? "…"
                  : s.value === undefined
                    ? "—"
                    : typeof s.value === "number"
                      ? s.value.toLocaleString()
                      : s.value}
              </strong>
              <small>{s.sub}</small>
            </CardContent>
          </Card>
        ))}
      </div>
      <div className="overview-columns">
        <section className="panel">
          <div className="panel-title">
            <h2>Recent buckets</h2>
            <button
              className="text-link"
              onClick={() => a.navigate("/buckets")}
            >
              View all <ArrowRight size={14} />
            </button>
          </div>
          {buckets.loading ? (
            <Loading />
          ) : buckets.error ? (
            <ErrorBox message={buckets.error} />
          ) : !buckets.data?.buckets?.length ? (
            <Empty title="Create your first bucket">
              Keep related objects together in a named storage space.
            </Empty>
          ) : (
            <div>
              {buckets.data.buckets
                .slice(-5)
                .reverse()
                .map((b: Data) => (
                  <button
                    className="bucket-row"
                    key={b.name}
                    onClick={() => a.navigate(`/browser/${enc(b.name)}`)}
                  >
                    <div className="row-icon">
                      <Database size={18} />
                    </div>
                    <div>
                      <strong>{b.name}</strong>
                      <small>Created {date(b.creation_date)}</small>
                    </div>
                    <ChevronRight size={16} />
                  </button>
                ))}
            </div>
          )}
        </section>
        <section className="panel">
          <div className="panel-title">
            <h2>Deployment</h2>
            <Badge variant="outline">Live</Badge>
          </div>
          {error ? (
            <div className="p-5">
              <ErrorBox message={error} />
              <p className="muted mt-3">
                Deployment metrics require server information permission.
              </p>
            </div>
          ) : loading ? (
            <Loading />
          ) : (
            <div className="deployment-list">
              <div>
                <span>Status</span>
                <strong>{data?.mode || "—"}</strong>
              </div>
              <div>
                <span>Region</span>
                <strong>{data?.region || "Default"}</strong>
              </div>
              {servers.map((s, i) => (
                <div key={i}>
                  <span className="truncate">{s.endpoint}</span>
                  <Badge
                    variant={
                      s.state === "ok" || s.state === "online"
                        ? "secondary"
                        : "outline"
                    }
                  >
                    {s.state || "Unknown"}
                  </Badge>
                </div>
              ))}
              <p className="muted">
                Usage is updated by the background scanner and may lag recent
                writes.
              </p>
            </div>
          )}
        </section>
      </div>
    </>
  );
}
export function Buckets(a: Actions) {
  const [rev, setRev] = useState(0),
    [filter, setFilter] = useState("");
  const { data, error, loading } = useData("/buckets", undefined, rev);
  function create() {
    a.edit({
      title: "Create bucket",
      description:
        "Choose a unique name for this storage space. Names must follow S3 bucket naming rules.",
      fields: [
        {
          name: "name",
          label: "Bucket name",
          required: true,
          placeholder: "e.g. project-assets",
        },
      ],
      submit: "Create bucket",
      run: async (v) => {
        await api("/buckets", "POST", { name: v.name });
        a.navigate(`/browser/${enc(v.name)}`);
      },
    });
  }
  return (
    <>
      <Header
        eyebrow="OBJECT STORAGE"
        title="Buckets"
        description="Organize, protect, and explore your stored objects."
      >
        <Button
          variant="outline"
          size="icon"
          aria-label="Refresh buckets"
          onClick={() => setRev((x) => x + 1)}
        >
          <RefreshCw size={16} />
        </Button>
        {can(a.session, "s3:CreateBucket") && (
          <Button onClick={create}>
            <Plus size={16} />
            Create bucket
          </Button>
        )}
      </Header>
      <section className="panel">
        <div className="table-toolbar">
          <div className="search">
            <Search size={16} />
            <Input
              aria-label="Search buckets"
              placeholder="Search buckets…"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
          </div>
          <span className="muted">{data?.buckets?.length ?? 0} buckets</span>
        </div>
        <ErrorBox message={error} />
        {loading ? (
          <Loading />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Bucket name</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Access</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(data?.buckets || [])
                .filter((b: Data) => b.name.includes(filter))
                .map((b: Data) => (
                  <TableRow key={b.name}>
                    <TableCell>
                      <button
                        className="name-cell"
                        onClick={() => a.navigate(`/browser/${enc(b.name)}`)}
                      >
                        <Database size={19} />
                        <strong>{b.name}</strong>
                      </button>
                    </TableCell>
                    <TableCell className="muted whitespace-nowrap">
                      {date(b.creation_date)}
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">
                        {b.access?.toLowerCase() || "Policy controlled"}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={`Open ${b.name}`}
                        onClick={() => a.navigate(`/browser/${enc(b.name)}`)}
                      >
                        <ArrowRight size={16} />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        )}
        {!loading && !error && !data?.buckets?.length && (
          <Empty title="No buckets yet">
            Create a bucket to start uploading objects.
          </Empty>
        )}
      </section>
    </>
  );
}
export function ObjectBrowser(a: Actions & { bucket: string; prefix: string }) {
  const [rev, setRev] = useState(0),
    [limit, setLimit] = useState(500),
    [filter, setFilter] = useState(""),
    [versions, setVersions] = useState(false),
    [selected, setSelected] = useState<Data | null>(null),
    [checked, setChecked] = useState<string[]>([]),
    [transfer, setTransfer] = useState(""),
    [transferError, setTransferError] = useState(""),
    [rewind, setRewind] = useState("");
  const fileRef = useRef<HTMLInputElement>(null),
    folderRef = useRef<HTMLInputElement>(null);
  const route = `/buckets/${enc(a.bucket)}`;
  const { data, error, loading } = useData(
    rewind
      ? `${route}/rewind/${enc(new Date(rewind).toISOString())}`
      : `${route}/objects`,
    { prefix: a.prefix, with_versions: versions, limit },
    rev,
  );
  useEffect(() => {
    setChecked([]);
    setFilter("");
    setLimit(500);
    setSelected(null);
  }, [a.bucket, a.prefix]);
  const objects: Data[] = data?.objects || [];
  const visible = objects.filter((o) =>
    (o.name || "")
      .slice(a.prefix.length)
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  const location = (prefix: string) =>
    a.navigate(`/browser/${enc(a.bucket)}?prefix=${enc(prefix)}`);
  function remove(items: Data[]) {
    const label =
      items.length === 1 ? items[0].name : `${items.length} objects`;
    a.edit({
      title: "Delete objects",
      description: `Delete ${label}? Folder deletion also removes objects under that prefix. Versioned buckets retain prior versions unless a specific version is selected.`,
      fields: [],
      danger: true,
      confirm: items.length === 1 ? items[0].name : "delete",
      submit: "Delete",
      run: async () => {
        for (const o of items)
          await api(`${route}/objects`, "DELETE", undefined, {
            prefix: o.name,
            version_id: versions ? o.version_id : undefined,
            recursive: o.name.endsWith("/"),
          });
      },
      onSuccess: () => {
        setRev((x) => x + 1);
        setChecked([]);
      },
    });
  }
  async function upload(files: File[]) {
    setTransferError("");
    let failures: string[] = [];
    for (let i = 0; i < files.length; i++) {
      const file = files[i],
        name = a.prefix + (file.webkitRelativePath || file.name);
      setTransfer(`Uploading ${i + 1} of ${files.length} · ${file.name}`);
      const form = new FormData();
      form.append(String(file.size), file, file.name);
      try {
        await api(`${route}/objects/upload`, "POST", form, { prefix: name });
      } catch (e) {
        failures.push(`${name}: ${(e as Error).message}`);
      }
    }
    setTransfer("");
    setTransferError(failures.join("\n"));
    setRev((x) => x + 1);
    a.notify(
      `${files.length - failures.length} of ${files.length} files uploaded`,
    );
  }
  async function downloadZip() {
    setTransfer("Preparing ZIP download…");
    setTransferError("");
    try {
      const response = await fetch(url(`${route}/objects/download-multiple`), {
        method: "POST",
        credentials: "same-origin",
        headers: {
          "Content-Type": "application/json",
          ...(a.session.anonymous ? { "X-Anonymous": "1" } : {}),
        },
        body: JSON.stringify(checked),
      });
      if (!response.ok)
        throw new Error(
          (await response.json()).message || "ZIP download failed",
        );
      const blob = await response.blob(),
        link = document.createElement("a"),
        blobURL = URL.createObjectURL(blob);
      link.href = blobURL;
      link.download = `${a.bucket}-objects.zip`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(blobURL), 60000);
    } catch (e) {
      setTransferError((e as Error).message);
    } finally {
      setTransfer("");
    }
  }
  function choose(files: FileList | null) {
    if (!files?.length) return;
    const list = Array.from(files);
    a.edit({
      title: "Upload objects",
      successMessage: false,
      description: `Upload ${list.length} file${list.length === 1 ? "" : "s"} (${bytes(list.reduce((n, f) => n + f.size, 0))}) to ${a.bucket}/${a.prefix}? An existing key is replaced; previous versions are kept only if bucket versioning is enabled.`,
      fields: [],
      submit: "Upload",
      run: async () => {
        void upload(list);
      },
    });
    if (fileRef.current) fileRef.current.value = "";
    if (folderRef.current) folderRef.current.value = "";
  }
  return (
    <>
      <div className="breadcrumbs">
        <button onClick={() => a.navigate("/buckets")}>Buckets</button>
        <ChevronRight size={14} />
        <button onClick={() => location("")}>{a.bucket}</button>
        {a.prefix
          .split("/")
          .filter(Boolean)
          .map((p, i, parts) => (
            <span key={i}>
              <ChevronRight size={14} />
              <button
                onClick={() => location(parts.slice(0, i + 1).join("/") + "/")}
              >
                {p}
              </button>
            </span>
          ))}
      </div>
      <Header
        title={
          a.prefix ? a.prefix.split("/").filter(Boolean).at(-1)! : a.bucket
        }
        description="Browse objects and manage their complete version history."
      >
        <Button
          variant="outline"
          onClick={() => a.navigate(`/buckets/${enc(a.bucket)}/settings`)}
        >
          <Settings2 size={16} />
          Settings
        </Button>
        {can(a.session, "s3:PutObject", `${a.bucket}/${a.prefix}*`) && (
          <>
            <Button
              disabled={!!transfer || !!rewind}
              onClick={() => fileRef.current?.click()}
            >
              <ArrowUpToLine size={16} />
              Upload files
            </Button>
            <Button
              disabled={!!transfer || !!rewind}
              variant="outline"
              onClick={() => folderRef.current?.click()}
            >
              Upload folder
            </Button>
          </>
        )}
      </Header>
      <input
        hidden
        type="file"
        multiple
        ref={fileRef}
        onChange={(e) => choose(e.target.files)}
      />
      <input
        hidden
        type="file"
        multiple
        ref={folderRef}
        {...{ webkitdirectory: "" }}
        onChange={(e) => choose(e.target.files)}
      />
      {transfer && (
        <div className="transfer">
          <RefreshCw size={16} className="animate-spin" />
          {transfer}
        </div>
      )}
      <ErrorBox message={transferError} />
      <section className="panel">
        <div className="table-toolbar">
          <div className="search">
            <Search size={16} />
            <Input
              aria-label="Filter objects"
              placeholder="Filter loaded objects…"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
          </div>
          <div className="toolbar-actions">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={versions}
                onChange={(e) => {
                  setVersions(e.target.checked);
                  setChecked([]);
                }}
              />
              Versions
            </label>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Rewind bucket"
              onClick={() =>
                a.edit({
                  title: "Rewind bucket",
                  description:
                    "View objects at a previous point in time. This does not modify storage.",
                  fields: [
                    {
                      name: "date",
                      label: "Point in time",
                      type: "datetime-local",
                      required: true,
                    },
                  ],
                  submit: "View snapshot",
                  run: async (v) => {
                    setRewind(v.date);
                  },
                })
              }
            >
              <Clock3 size={17} />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="Refresh objects"
              onClick={() => setRev((x) => x + 1)}
            >
              <RefreshCw size={16} />
            </Button>
          </div>
        </div>
        {rewind && (
          <div className="info-bar">
            Snapshot: {date(rewind)}{" "}
            <Button variant="ghost" size="sm" onClick={() => setRewind("")}>
              Return to current objects
            </Button>
          </div>
        )}
        {checked.length > 0 && (
          <div className="info-bar">
            <strong>{checked.length} selected</strong>
            <Button
              size="sm"
              variant="outline"
              disabled={!!transfer}
              onClick={() =>
                a.edit({
                  title: "Download selected objects",
                  successMessage: false,
                  description:
                    "Create a ZIP archive of the selected objects. Folders include their contents. Large archives use browser memory; use mc for very large transfers.",
                  fields: [],
                  submit: "Download ZIP",
                  run: async () => {
                    void downloadZip();
                  },
                })
              }
            >
              <ArrowDownToLine size={14} />
              Download ZIP
            </Button>
            <Button
              size="sm"
              variant="destructive"
              disabled={
                !checked.every((name) =>
                  can(a.session, "s3:DeleteObject", `${a.bucket}/${name}`),
                )
              }
              onClick={() =>
                remove(objects.filter((o) => checked.includes(o.name)))
              }
            >
              Delete selected
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setChecked([])}>
              Clear
            </Button>
          </div>
        )}
        <ErrorBox message={error} />
        {loading ? (
          <Loading />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10" />
                <TableHead>Object name</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Last modified</TableHead>
                {versions && <TableHead>Version</TableHead>}
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((o, i) => (
                <TableRow key={`${o.name}-${o.version_id}-${i}`}>
                  <TableCell>
                    {!versions &&
                      !rewind &&
                      (can(
                        a.session,
                        "s3:GetObject",
                        `${a.bucket}/${o.name}`,
                      ) ||
                        can(
                          a.session,
                          "s3:DeleteObject",
                          `${a.bucket}/${o.name}`,
                        )) && (
                        <input
                          aria-label={`Select ${o.name}`}
                          type="checkbox"
                          checked={checked.includes(o.name)}
                          onChange={(e) =>
                            setChecked(
                              e.target.checked
                                ? [...checked, o.name]
                                : checked.filter((x) => x !== o.name),
                            )
                          }
                        />
                      )}
                  </TableCell>
                  <TableCell>
                    <button
                      className="name-cell"
                      onClick={() =>
                        o.name.endsWith("/") && !versions
                          ? location(o.name)
                          : setSelected(o)
                      }
                    >
                      {o.name.endsWith("/") ? (
                        <Folder size={19} className="folder-icon" />
                      ) : (
                        <File size={18} />
                      )}
                      <span>{o.name.slice(a.prefix.length) || o.name}</span>
                      {(o.is_delete_marker || o.delete_flag) && (
                        <Badge variant="outline">Deleted</Badge>
                      )}
                    </button>
                  </TableCell>
                  <TableCell className="muted whitespace-nowrap">
                    {o.name.endsWith("/") ? "—" : bytes(o.size)}
                  </TableCell>
                  <TableCell className="muted whitespace-nowrap">
                    {date(o.last_modified)}
                  </TableCell>
                  {versions && (
                    <TableCell className="version-cell">
                      {o.is_latest && <Badge variant="secondary">Latest</Badge>}
                      <code>{o.version_id}</code>
                    </TableCell>
                  )}
                  <TableCell>
                    <div className="row-actions">
                      {!o.name.endsWith("/") &&
                        !o.is_delete_marker &&
                        !o.delete_flag && (
                          <Button asChild size="icon" variant="ghost">
                            <a
                              aria-label={`Download ${o.name}`}
                              href={url(`${route}/objects/download`, {
                                prefix: o.name,
                                version_id: o.version_id,
                              })}
                              download
                              onClick={async (event) => {
                                if (!a.session.anonymous) return;
                                event.preventDefault();
                                try {
                                  const response = await fetch(
                                    event.currentTarget.href,
                                    {
                                      credentials: "same-origin",
                                      headers: { "X-Anonymous": "1" },
                                    },
                                  );
                                  if (!response.ok)
                                    throw new Error("Download failed");
                                  const objectURL = URL.createObjectURL(
                                      await response.blob(),
                                    ),
                                    link = document.createElement("a");
                                  link.href = objectURL;
                                  link.download =
                                    o.name.split("/").at(-1) || "object";
                                  link.click();
                                  setTimeout(
                                    () => URL.revokeObjectURL(objectURL),
                                    60000,
                                  );
                                } catch (e) {
                                  setTransferError((e as Error).message);
                                }
                              }}
                            >
                              <ArrowDownToLine size={16} />
                            </a>
                          </Button>
                        )}
                      {!rewind &&
                        can(
                          a.session,
                          versions
                            ? "s3:DeleteObjectVersion"
                            : "s3:DeleteObject",
                          `${a.bucket}/${o.name}`,
                        ) && (
                          <Button
                            aria-label={`Delete ${o.name}`}
                            variant="ghost"
                            size="icon"
                            onClick={() => remove([o])}
                          >
                            <Trash2 size={15} />
                          </Button>
                        )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        {!loading && !error && !visible.length && (
          <Empty
            title={filter ? "No matching objects" : "This folder is empty"}
          >
            {filter
              ? "Try a different filter."
              : "Upload files to add objects to this location."}
          </Empty>
        )}
        <div className="table-footer">
          <span>
            {visible.length} objects shown
            {objects.length >= limit ? " · More may be available" : ""}
          </span>
          {objects.length >= limit && !rewind && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setLimit((x) => x + 500)}
            >
              Load more
            </Button>
          )}
        </div>
      </section>
      {selected && (
        <ObjectDetail
          {...a}
          bucket={a.bucket}
          object={selected}
          close={() => setSelected(null)}
          refresh={() => {
            setRev((x) => x + 1);
            setSelected(null);
          }}
        />
      )}
    </>
  );
}
function ObjectDetail(
  a: Actions & {
    bucket: string;
    object: Data;
    close: () => void;
    refresh: () => void;
  },
) {
  const o = a.object,
    route = `/buckets/${enc(a.bucket)}/objects`;
  const meta = useData(`${route}/metadata`, {
      prefix: o.name,
      versionID: o.version_id,
    }),
    versions = useData(route, {
      prefix: o.name,
      with_versions: true,
      limit: 1000,
    });
  const [share, setShare] = useState(""),
    [error, setError] = useState(""),
    [preview, setPreview] = useState(""),
    [imagePreview, setImagePreview] = useState("");
  useEffect(
    () => () => {
      if (imagePreview) URL.revokeObjectURL(imagePreview);
    },
    [imagePreview],
  );
  const detail =
    (versions.data?.objects || []).find(
      (v: Data) =>
        v.name === o.name &&
        (o.version_id
          ? v.version_id === o.version_id
          : v.is_latest || !v.version_id),
    ) || o;
  const tags = detail.tags || detail.user_tags || {};
  async function anonymousDownload(event: React.MouseEvent<HTMLAnchorElement>) {
    if (!a.session.anonymous) return;
    event.preventDefault();
    try {
      const response = await fetch(event.currentTarget.href, {
        credentials: "same-origin",
        headers: { "X-Anonymous": "1" },
      });
      if (!response.ok) throw new Error("Download failed");
      const objectURL = URL.createObjectURL(await response.blob()),
        link = document.createElement("a");
      link.href = objectURL;
      link.download = o.name.split("/").at(-1) || "object";
      link.click();
      setTimeout(() => URL.revokeObjectURL(objectURL), 60000);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && a.close()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="break-all">{o.name}</DialogTitle>
          <DialogDescription>
            Object details · {bytes(o.size)}
          </DialogDescription>
        </DialogHeader>
        <ErrorBox message={error} />
        <div className="detail-actions">
          <Button asChild variant="outline">
            <a
              href={url(`${route}/download`, {
                prefix: o.name,
                version_id: o.version_id,
              })}
              download
              onClick={anonymousDownload}
            >
              <ArrowDownToLine size={16} />
              Download
            </a>
          </Button>
          <Button
            variant="outline"
            onClick={async () => {
              try {
                const link = await api<string>(
                  `${route}/share`,
                  "GET",
                  undefined,
                  {
                    prefix: o.name,
                    version_id: o.version_id || "null",
                    expires: "1h",
                  },
                );
                setShare(link);
              } catch (e) {
                setError((e as Error).message);
              }
            }}
          >
            <Link2 size={16} />
            Share for 1 hour
          </Button>
          {Number(o.size) <= 1048576 &&
            /\.(txt|json|csv|md|log|xml|ya?ml)$/i.test(o.name) && (
              <Button
                variant="outline"
                onClick={async () => {
                  try {
                    const response = await fetch(
                      url(`${route}/download`, {
                        prefix: o.name,
                        version_id: o.version_id,
                      }),
                      {
                        credentials: "same-origin",
                        headers: a.session.anonymous
                          ? { "X-Anonymous": "1" }
                          : {},
                      },
                    );
                    if (!response.ok)
                      throw new Error("Preview could not be loaded");
                    setPreview(await response.text());
                  } catch (e) {
                    setError((e as Error).message);
                  }
                }}
              >
                Preview text
              </Button>
            )}
        </div>
        {share && (
          <div className="field">
            <label htmlFor="share-link">
              Anyone with this link can download the object until it expires.
            </label>
            <Input
              id="share-link"
              readOnly
              value={share}
              onFocus={(e) => e.target.select()}
            />
          </div>
        )}
        {preview && <pre className="code-preview">{preview}</pre>}
        {Number(o.size) <= 20971520 &&
          /\.(png|jpe?g|webp|gif|avif)$/i.test(o.name) && (
            <Button
              variant="outline"
              onClick={async () => {
                try {
                  const response = await fetch(
                    url(`${route}/download`, {
                      prefix: o.name,
                      version_id: o.version_id,
                    }),
                    {
                      credentials: "same-origin",
                      headers: a.session.anonymous
                        ? { "X-Anonymous": "1" }
                        : {},
                    },
                  );
                  if (!response.ok) throw new Error("Image preview failed");
                  setImagePreview(URL.createObjectURL(await response.blob()));
                } catch (e) {
                  setError((e as Error).message);
                }
              }}
            >
              Preview image
            </Button>
          )}
        {imagePreview && (
          <img
            src={imagePreview}
            alt={o.name}
            className="max-h-96 w-full object-contain rounded-lg bg-muted"
          />
        )}
        <div className="detail-grid">
          <div>
            <small>Last modified</small>
            <strong>{date(o.last_modified)}</strong>
          </div>
          <div>
            <small>ETag</small>
            <code>{o.etag || "—"}</code>
          </div>
          <div>
            <small>Content type</small>
            <strong>{o.content_type || "—"}</strong>
          </div>
          <div>
            <small>Version</small>
            <code>{o.version_id || "Unversioned"}</code>
          </div>
        </div>
        {(detail.retention_mode || detail.legal_hold_status) && (
          <div className="info-bar">
            Retention: {detail.retention_mode || "None"}
            {detail.retention_until_date
              ? ` until ${date(detail.retention_until_date)}`
              : ""}{" "}
            · Legal hold: {detail.legal_hold_status || "Off"}
          </div>
        )}
        <h3 className="section-label">Metadata</h3>
        <ErrorBox message={meta.error} />
        {meta.loading ? (
          <Loading />
        ) : (
          <dl className="metadata">
            {Object.entries(meta.data?.objectMetadata || {}).map(
              ([key, value]) => (
                <div key={key}>
                  <dt>{key}</dt>
                  <dd>
                    {typeof value === "object"
                      ? JSON.stringify(value)
                      : String(value)}
                  </dd>
                </div>
              ),
            )}
          </dl>
        )}
        <div className="panel-title p-0">
          <h3 className="section-label">Tags</h3>
          {can(a.session, "s3:PutObjectTagging", `${a.bucket}/${o.name}`) &&
            can(a.session, "s3:GetObjectTagging", `${a.bucket}/${o.name}`) && (
              <Button
                size="sm"
                variant="outline"
                disabled={versions.loading || !!versions.error}
                onClick={() =>
                  a.edit({
                    title: "Edit object tags",
                    description:
                      "Tags are key/value labels attached to this object version.",
                    fields: [
                      {
                        name: "tags",
                        label: "Tags (JSON object)",
                        type: "textarea",
                        value: JSON.stringify(tags, null, 2),
                      },
                    ],
                    run: (v) =>
                      api(
                        `${route}/tags`,
                        "PUT",
                        { tags: JSON.parse(v.tags) },
                        { prefix: o.name, version_id: o.version_id || "null" },
                      ),
                    onSuccess: a.refresh,
                  })
                }
              >
                Edit tags
              </Button>
            )}
        </div>
        <div className="tag-list">
          {Object.entries(tags).map(([k, v]) => (
            <Badge key={k} variant="secondary">
              {k}: {String(v)}
            </Badge>
          ))}
          {!Object.keys(tags).length && <p className="muted">No tags</p>}
        </div>
        <h3 className="section-label">Version history</h3>
        <ErrorBox message={versions.error} />
        {versions.loading ? (
          <Loading />
        ) : (
          <div className="version-list">
            {(versions.data?.objects || [])
              .filter((v: Data) => v.name === o.name)
              .map((v: Data, i: number) => (
                <div key={i}>
                  <div>
                    <strong>{date(v.last_modified)}</strong>
                    <code>
                      {v.version_id || "Unversioned"}
                      {v.is_latest ? " · Latest" : ""}
                      {v.is_delete_marker ? " · Delete marker" : ""}
                    </code>
                  </div>
                  {v.version_id &&
                    !v.is_latest &&
                    !v.is_delete_marker &&
                    can(a.session, "s3:PutObject", `${a.bucket}/${o.name}`) && (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() =>
                          a.edit({
                            title: "Restore object version",
                            description:
                              "Copy this version to become the latest version. The existing version history is retained.",
                            fields: [],
                            submit: "Restore version",
                            run: () =>
                              api(`${route}/restore`, "PUT", undefined, {
                                prefix: o.name,
                                version_id: v.version_id,
                              }),
                            onSuccess: a.refresh,
                          })
                        }
                      >
                        Restore
                      </Button>
                    )}
                </div>
              ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
