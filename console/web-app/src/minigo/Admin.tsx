import { useState } from "react";
import {
  ChevronRight,
  KeyRound,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  Shield,
  Trash2,
  UserRound,
  UsersRound,
  Server,
  CircleHelp,
} from "lucide-react";
import { api, bytes, can, date, enc, type Data } from "./api";
import {
  Button,
  Input,
  Empty,
  ErrorBox,
  Header,
  Loading,
  useData,
  type Actions,
  type Field,
} from "./shared";
import { Badge } from "./components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./components/ui/table";
const parseList = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
export function Identity(
  a: Actions & { kind: "users" | "groups" | "policies" | "service-accounts" },
) {
  const [rev, setRev] = useState(0),
    [search, setSearch] = useState(""),
    [error, setError] = useState(""),
    [secret, setSecret] = useState<Data | null>(null);
  const {
    data,
    error: loadError,
    loading,
  } = useData(`/minigo/${a.kind}`, undefined, rev);
  const refresh = () => setRev((x) => x + 1),
    isUser = a.kind === "users",
    isGroup = a.kind === "groups",
    isPolicy = a.kind === "policies",
    isKey = a.kind === "service-accounts";
  const singular = isUser
    ? "user"
    : isGroup
      ? "group"
      : isPolicy
        ? "policy"
        : "access key";
  const title = isKey
    ? "Access keys"
    : a.kind[0].toUpperCase() + a.kind.slice(1);
  const Icon = isUser
    ? UserRound
    : isGroup
      ? UsersRound
      : isPolicy
        ? Shield
        : KeyRound;
  const rows: Data[] = isGroup
    ? (data?.groups || []).map((name: string) => ({ name }))
    : data?.[isKey ? "accounts" : a.kind] || [];
  const path = (name: string) => `/minigo/${a.kind}/${enc(name)}`;
  const creationPermission = isUser
    ? "admin:CreateUser"
    : isGroup
      ? "admin:AddUserToGroup"
      : isPolicy
        ? "admin:CreatePolicy"
        : "admin:CreateServiceAccount";
  const deletionPermission = isUser
    ? "admin:DeleteUser"
    : isGroup
      ? "admin:RemoveUserFromGroup"
      : isPolicy
        ? "admin:DeletePolicy"
        : "admin:RemoveServiceAccount";
  function create() {
    const fields: Field[] = isUser
      ? [
          { name: "name", label: "Username", required: true },
          {
            name: "secretKey",
            label: "Password",
            type: "password",
            required: true,
            help: "Use a unique, strong password of at least 8 characters.",
          },
        ]
      : isGroup
        ? [
            { name: "name", label: "Group name", required: true },
            {
              name: "members",
              label: "Members",
              help: "Existing usernames, separated by commas.",
              required: true,
            },
          ]
        : isPolicy
          ? [
              { name: "name", label: "Policy name", required: true },
              {
                name: "document",
                label: "IAM policy document",
                type: "textarea",
                value: JSON.stringify(
                  { Version: "2012-10-17", Statement: [] },
                  null,
                  2,
                ),
                required: true,
              },
            ]
          : [
              { name: "name", label: "Name", required: true },
              { name: "description", label: "Description" },
              {
                name: "policy",
                label: "Optional restricted IAM policy",
                type: "textarea",
                help: "Leave empty to inherit the parent user permissions.",
              },
              {
                name: "expiration",
                label: "Expiration",
                type: "datetime-local",
                help: "Optional. Keep access keys short lived where possible.",
              },
            ];
    a.edit({
      title: `Create ${singular}`,
      description: isKey
        ? "Generate credentials for an application. The secret is shown only once."
        : isPolicy
          ? "Define the S3 and administration actions this policy allows. New policies do not grant access until attached."
          : `Add a ${singular} to this deployment. Access is controlled through attached policies.`,
      fields,
      submit: `Create ${singular}`,
      run: async (v) => {
        if (isUser)
          return api(path(v.name), "PUT", {
            secretKey: v.secretKey,
            status: "enabled",
          });
        if (isGroup)
          return api(path(v.name), "PUT", { members: parseList(v.members) });
        if (isPolicy)
          return api(path(v.name), "PUT", { document: JSON.parse(v.document) });
        const result = await api("/minigo/service-accounts", "POST", {
          name: v.name,
          description: v.description,
          policy: v.policy ? JSON.parse(v.policy) : undefined,
          expiration: v.expiration
            ? new Date(v.expiration).toISOString()
            : undefined,
        });
        setSecret(result);
      },
      onSuccess: refresh,
    });
  }
  async function modify(row: Data) {
    const name = row.accessKey || row.name;
    setError("");
    try {
      const detail = await api(path(name));
      if (isPolicy) {
        a.edit({
          title: "Edit policy",
          description: `Replace the policy document for ${name}. Changes immediately affect all attached users and groups.`,
          fields: [
            {
              name: "document",
              label: "IAM policy document",
              type: "textarea",
              value: JSON.stringify(detail.document, null, 2),
              required: true,
            },
          ],
          confirm: name,
          run: (v) =>
            api(path(name), "PUT", { document: JSON.parse(v.document) }),
          onSuccess: refresh,
        });
      } else if (isGroup) {
        a.edit({
          title: "Edit group members",
          description: `Update membership for ${name}. Existing members omitted from this list will be removed.`,
          fields: [
            {
              name: "members",
              label: "Members",
              value: (detail.members || []).join(", "),
              help: "Usernames, separated by commas.",
            },
          ],
          run: async (v) => {
            const members = parseList(v.members),
              removed = (detail.members || []).filter(
                (x: string) => !members.includes(x),
              );
            if (members.length) await api(path(name), "PUT", { members });
            if (removed.length)
              await api(path(name), "PUT", { members: removed, remove: true });
          },
          onSuccess: refresh,
        });
      } else if (isUser) {
        a.edit({
          title: "Reset user password",
          description: `Change the password for ${name}. Applications using these credentials must be updated.`,
          fields: [
            {
              name: "secretKey",
              label: "New password",
              type: "password",
              required: true,
            },
          ],
          confirm: name,
          run: (v) =>
            api(path(name), "PUT", {
              secretKey: v.secretKey,
              status: detail.status,
            }),
          onSuccess: refresh,
        });
      } else {
        a.edit({
          title: "Edit access key",
          description:
            "Update this application credential. Secret rotation is managed separately with mc.",
          fields: [
            { name: "name", label: "Name", value: detail.name || row.name },
            {
              name: "description",
              label: "Description",
              value: detail.description || row.description,
            },
            {
              name: "status",
              label: "Status",
              value: detail.accountStatus || "on",
              options: ["on", "off"],
            },
          ],
          run: (v) =>
            api(path(row.accessKey), "PATCH", {
              newName: v.name,
              newDescription: v.description,
              newStatus: v.status,
            }),
          onSuccess: refresh,
        });
      }
    } catch (e) {
      setError((e as Error).message);
    }
  }
  function remove(row: Data) {
    const name = row.accessKey || row.name;
    a.edit({
      title: `Delete ${singular}`,
      description: isGroup
        ? "Only an empty group can be deleted. Remove its members first."
        : `Permanently remove ${name}. Any dependent applications or attached permissions may stop working.`,
      fields: [],
      danger: true,
      confirm: name,
      submit: "Delete",
      run: () => api(path(name), "DELETE"),
      onSuccess: refresh,
    });
  }
  function associate(row: Data) {
    a.edit({
      title: "Assign policies",
      description: `Attach or detach existing policies for ${row.accessKey || row.name}. Policy permissions take effect immediately.`,
      fields: [
        {
          name: "policies",
          label: "Policy names",
          required: true,
          help: "Existing policy names, separated by commas.",
        },
        {
          name: "action",
          label: "Action",
          value: "attach",
          options: ["attach", "detach"],
        },
      ],
      run: (v) =>
        api("/minigo/policy-associations", "POST", {
          policies: parseList(v.policies),
          [isUser ? "user" : "group"]: row.accessKey || row.name,
          action: v.action,
        }),
      onSuccess: refresh,
    });
  }
  return (
    <>
      <Header
        eyebrow="ACCESS MANAGEMENT"
        title={title}
        description={
          isUser
            ? "Manage identities and their access to storage."
            : isGroup
              ? "Organize users into groups with shared permissions."
              : isPolicy
                ? "Define exactly who can do what across your deployment."
                : "Application credentials with controlled permissions and expiration."
        }
      >
        <Button
          variant="outline"
          size="icon"
          aria-label={`Refresh ${title}`}
          onClick={refresh}
        >
          <RefreshCw size={16} />
        </Button>
        {can(a.session, creationPermission) && (
          <Button onClick={create}>
            <Plus size={16} />
            Create {singular}
          </Button>
        )}
      </Header>
      {secret && (
        <section className="secret-panel">
          <h2>Save your new credentials</h2>
          <p>
            The secret key will not be shown again. Store it securely before
            dismissing.
          </p>
          <div className="field">
            <label htmlFor="new-access-key">Access key</label>
            <Input id="new-access-key" readOnly value={secret.accessKey} />
          </div>
          <div className="field">
            <label htmlFor="new-secret-key">Secret key</label>
            <Input
              id="new-secret-key"
              readOnly
              value={secret.secretKey}
              type="password"
              onFocus={(e) => (e.currentTarget.type = "text")}
              onBlur={(e) => (e.currentTarget.type = "password")}
            />
          </div>
          <Button variant="outline" onClick={() => setSecret(null)}>
            I have saved these credentials
          </Button>
        </section>
      )}
      <ErrorBox message={error} />
      <section className="panel">
        <div className="table-toolbar">
          <div className="search">
            <Search size={16} />
            <Input
              aria-label={`Search ${title}`}
              placeholder={`Search ${title.toLowerCase()}…`}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <span className="muted">
            {rows.length} {title.toLowerCase()}
          </span>
        </div>
        <ErrorBox message={loadError} />
        {loading ? (
          <Loading />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{isKey ? "Access key" : "Name"}</TableHead>
                {!isPolicy && (
                  <TableHead>{isGroup ? "Membership" : "Status"}</TableHead>
                )}
                <TableHead>
                  {isUser
                    ? "Policies"
                    : isKey
                      ? "Description"
                      : isPolicy
                        ? "Statements"
                        : "Policy management"}
                </TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows
                .filter((r) =>
                  (r.accessKey || r.name)
                    .toLowerCase()
                    .includes(search.toLowerCase()),
                )
                .map((row) => (
                  <TableRow key={row.accessKey || row.name}>
                    <TableCell>
                      <button className="name-cell" onClick={() => modify(row)}>
                        <Icon size={18} />
                        <strong>{row.accessKey || row.name}</strong>
                      </button>
                    </TableCell>
                    {!isPolicy && (
                      <TableCell>
                        {isGroup ? (
                          <span className="muted">Manage members</span>
                        ) : (
                          <Badge variant="secondary">
                            {row.status || row.accountStatus || "Unknown"}
                          </Badge>
                        )}
                      </TableCell>
                    )}
                    <TableCell className="muted">
                      {isUser
                        ? row.policyName || "No direct policy"
                        : isKey
                          ? row.description || row.name || "—"
                          : isPolicy
                            ? `${row.document?.Statement?.length ?? 0} statements`
                            : "Policies apply to all members"}
                    </TableCell>
                    <TableCell>
                      <div className="row-actions">
                        {(isUser || isGroup) &&
                          can(a.session, "admin:AttachUserOrGroupPolicy") && (
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => associate(row)}
                            >
                              Policies
                            </Button>
                          )}
                        {isUser &&
                          can(
                            a.session,
                            row.status === "enabled"
                              ? "admin:DisableUser"
                              : "admin:EnableUser",
                          ) && (
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() =>
                                a.edit({
                                  title:
                                    row.status === "enabled"
                                      ? "Disable user"
                                      : "Enable user",
                                  description: `Change access for ${row.accessKey}. Disabling access may interrupt applications using this identity.`,
                                  fields: [],
                                  confirm: row.accessKey,
                                  run: () =>
                                    api(path(row.accessKey), "PATCH", {
                                      status:
                                        row.status === "enabled"
                                          ? "disabled"
                                          : "enabled",
                                    }),
                                  onSuccess: refresh,
                                })
                              }
                            >
                              {row.status === "enabled" ? "Disable" : "Enable"}
                            </Button>
                          )}
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={`Edit ${row.accessKey || row.name}`}
                          onClick={() => modify(row)}
                        >
                          <Settings2 size={16} />
                        </Button>
                        {can(a.session, deletionPermission) && (
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={`Delete ${row.accessKey || row.name}`}
                            onClick={() => remove(row)}
                          >
                            <Trash2 size={16} />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        )}
        {!loading && !loadError && !rows.length && (
          <Empty title={`No ${title.toLowerCase()} yet`}>
            Create a {singular} to get started.
          </Empty>
        )}
      </section>
      <div className="help-note">
        <CircleHelp size={17} />
        <span>
          Access changes are enforced by the server. Available actions depend on
          your current policy permissions.
        </span>
      </div>
    </>
  );
}
export function BucketSettings(a: Actions & { bucket: string }) {
  const [rev, setRev] = useState(0),
    [tab, setTab] = useState("versioning"),
    [error, setError] = useState("");
  const route = `/minigo/buckets/${enc(a.bucket)}`;
  const {
    data: configuration,
    error: configurationError,
    errorCode,
    loading,
  } = useData(`${route}/${tab}`, undefined, rev);
  // An absent rule set is editable; a missing bucket or denied access is not.
  const missingConfiguration =
    (tab === "lifecycle" && errorCode === "NoSuchLifecycleConfiguration") ||
    (tab === "replication" &&
      errorCode === "ReplicationConfigurationNotFoundError");
  const data = configuration || (missingConfiguration ? { Rules: [] } : null);
  const loadError = missingConfiguration ? "" : configurationError;
  const refresh = () => setRev((x) => x + 1);
  const putPermission =
    tab === "versioning"
      ? "s3:PutBucketVersioning"
      : tab === "lifecycle"
        ? "s3:PutLifecycleConfiguration"
        : "s3:PutReplicationConfiguration";
  function edit() {
    if (!data) return;
    if (tab === "versioning") {
      a.edit({
        title: "Change versioning",
        description:
          "Enabling versioning keeps previous copies of each object. Suspending it stops creating new versions but retains existing version history.",
        fields: [
          {
            name: "status",
            label: "Versioning status",
            value: data.Status === "Enabled" ? "Enabled" : "Suspended",
            options: ["Enabled", "Suspended"],
          },
        ],
        confirm: a.bucket,
        run: (v) =>
          api(`${route}/versioning`, "PUT", { ...data, Status: v.status }),
        onSuccess: refresh,
      });
    } else {
      a.edit({
        title: `Edit ${tab} rules`,
        description: `Replace the complete ${tab} configuration for ${a.bucket}. ${tab === "lifecycle" ? "Expiration rules can permanently remove stored data." : "Replication requires an existing destination target configured with mc."} Review every rule before saving.`,
        fields: [
          {
            name: "document",
            label: "Configuration (JSON)",
            type: "textarea",
            value: JSON.stringify(data, null, 2),
            required: true,
          },
        ],
        confirm: a.bucket,
        run: (v) => api(`${route}/${tab}`, "PUT", JSON.parse(v.document)),
        onSuccess: refresh,
      });
    }
  }
  return (
    <>
      <div className="breadcrumbs">
        <button onClick={() => a.navigate("/buckets")}>Buckets</button>
        <ChevronRight size={14} />
        <button onClick={() => a.navigate(`/browser/${enc(a.bucket)}`)}>
          {a.bucket}
        </button>
        <ChevronRight size={14} />
        <span>Settings</span>
      </div>
      <Header
        title="Bucket settings"
        description={`Protection and storage rules for ${a.bucket}.`}
      />
      <div className="tabs" role="tablist" aria-label="Bucket configuration">
        {["versioning", "lifecycle", "replication"].map((t) => (
          <button
            role="tab"
            aria-selected={tab === t}
            key={t}
            onClick={() => setTab(t)}
          >
            {t[0].toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>
      <section className="panel settings-panel">
        <div className="panel-title">
          <div>
            <h2>
              {tab === "versioning"
                ? "Keep a history of every object"
                : tab === "lifecycle"
                  ? "Manage objects over time"
                  : "Replicate to another destination"}
            </h2>
            <p className="muted">
              {tab === "versioning"
                ? "Recover from accidental replacements or deletions."
                : tab === "lifecycle"
                  ? "Control expiration and transition rules. Changes can remove data."
                  : "Keep objects synchronized with configured remote storage."}
            </p>
          </div>
          {can(a.session, putPermission, a.bucket) && (
            <Button
              variant="outline"
              disabled={loading || !!loadError}
              onClick={edit}
            >
              <Settings2 size={16} />
              Edit configuration
            </Button>
          )}
        </div>
        <ErrorBox message={loadError || error} />
        {loading ? (
          <Loading />
        ) : (
          data && (
            <div className="config-content">
              {tab === "versioning" ? (
                <div className="versioning-status">
                  <div className="row-icon">
                    <Shield size={22} />
                  </div>
                  <div>
                    <strong>{data.Status || "Not enabled"}</strong>
                    <p className="muted">
                      {data.Status === "Enabled"
                        ? "New writes preserve the previous object version."
                        : "New writes may replace unversioned objects."}
                    </p>
                  </div>
                  <Badge variant="secondary">
                    {data.Status === "Enabled"
                      ? "Protected"
                      : "Review recommended"}
                  </Badge>
                </div>
              ) : (
                <>
                  {(data.Rules || []).length ? (
                    <div className="rules">
                      {data.Rules.map((r: Data, i: number) => (
                        <div key={i}>
                          <strong>{r.ID || `Rule ${i + 1}`}</strong>
                          <Badge variant="secondary">
                            {r.Status || "Configured"}
                          </Badge>
                          <p className="muted">
                            Prefix:{" "}
                            {r.Filter?.Prefix || r.Prefix || "All objects"}
                            {r.Expiration?.Days
                              ? ` · Expires after ${r.Expiration.Days} days`
                              : ""}
                            {r.Destination?.Bucket
                              ? ` · Destination: ${r.Destination.Bucket}`
                              : ""}
                          </p>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <Empty title={`No ${tab} rules`}>
                      This bucket has no {tab} configuration.
                    </Empty>
                  )}
                  <details className="configuration-details">
                    <summary>Inspect configuration</summary>
                    <pre>{JSON.stringify(data, null, 2)}</pre>
                  </details>
                  {(data.Rules || []).length > 0 &&
                    can(a.session, putPermission, a.bucket) && (
                      <Button
                        variant="outline"
                        onClick={() =>
                          a.edit({
                            title: `Remove ${tab} configuration`,
                            description: `Remove all ${tab} rules from this bucket. Existing objects remain, but future ${tab} actions will stop.`,
                            fields: [],
                            confirm: a.bucket,
                            danger: true,
                            submit: "Remove configuration",
                            run: () => api(`${route}/${tab}`, "DELETE"),
                            onSuccess: refresh,
                          })
                        }
                      >
                        Remove all rules
                      </Button>
                    )}
                </>
              )}
            </div>
          )
        )}
      </section>
      {can(a.session, "s3:DeleteBucket", a.bucket) && (
        <section className="danger-zone">
          <div>
            <h2>Delete bucket</h2>
            <p>
              Only empty buckets can be deleted. Objects and all their versions
              must be removed first.
            </p>
          </div>
          <Button
            variant="outline"
            onClick={() =>
              a.edit({
                title: "Delete bucket",
                description: `Permanently delete ${a.bucket}. The server rejects this request if the bucket contains any objects or versions.`,
                fields: [],
                danger: true,
                confirm: a.bucket,
                submit: "Delete bucket",
                run: () => api(route, "DELETE"),
                onSuccess: () => a.navigate("/buckets"),
              })
            }
          >
            <Trash2 size={16} />
            Delete bucket
          </Button>
        </section>
      )}
    </>
  );
}
export function System(a: Actions) {
  const [rev, setRev] = useState(0),
    [subsystem, setSubsystem] = useState("api"),
    [restart, setRestart] = useState(false);
  const server = useData("/minigo/server", undefined, rev),
    config = useData(`/minigo/config/${enc(subsystem)}`, undefined, rev);
  return (
    <>
      <Header
        eyebrow="INFRASTRUCTURE"
        title="Deployment"
        description="Server health, disk capacity, and controlled configuration."
      >
        <Button variant="outline" onClick={() => setRev((x) => x + 1)}>
          <RefreshCw size={16} />
          Refresh
        </Button>
      </Header>
      <ErrorBox message={server.error} />
      {server.loading ? (
        <Loading />
      ) : (
        <section className="panel">
          <div className="panel-title">
            <h2>Servers & drives</h2>
            <Badge variant="outline">{server.data?.mode || "—"}</Badge>
          </div>
          <div className="server-list">
            {(server.data?.servers || []).map((s: Data, i: number) => (
              <div className="server-card" key={i}>
                <div className="server-title">
                  <Server size={19} />
                  <strong>{s.endpoint}</strong>
                  <Badge variant="secondary">{s.state}</Badge>
                </div>
                <div className="muted">
                  Version {s.version || "—"} · Up{" "}
                  {s.uptime ? `${Math.floor(s.uptime / 3600)} hours` : "—"}
                </div>
                {(s.drives || []).map((d: Data, j: number) => (
                  <div className="drive" key={j}>
                    <div>
                      <span>Drive {j + 1}</span>
                      <span>
                        {bytes(d.usedSpace)} / {bytes(d.totalSpace)} · {d.state}
                      </span>
                    </div>
                    <progress
                      aria-label={`Drive ${j + 1} usage`}
                      value={d.usedSpace || 0}
                      max={d.totalSpace || 1}
                    />
                  </div>
                ))}
              </div>
            ))}
          </div>
        </section>
      )}
      <section className="panel mt-6">
        <div className="panel-title">
          <div>
            <h2>Server configuration</h2>
            <p className="muted">
              Selected nonsecret operational settings. Some changes require a
              restart.
            </p>
          </div>
        </div>
        <div
          className="tabs config-tabs"
          role="tablist"
          aria-label="Configuration section"
        >
          {["api", "region", "scanner", "heal"].map((s) => (
            <button
              key={s}
              role="tab"
              aria-selected={s === subsystem}
              onClick={() => setSubsystem(s)}
            >
              {s === "api" ? "API" : s[0].toUpperCase() + s.slice(1)}
            </button>
          ))}
        </div>
        <ErrorBox message={config.error} />
        {restart && (
          <div className="info-bar">
            Configuration saved. Restart the deployment with your existing
            orchestration to apply this change.
          </div>
        )}
        {config.loading ? (
          <Loading />
        ) : (
          config.data && (
            <div className="config-content">
              <dl className="metadata">
                {Object.entries(config.data.values || {}).map(([k, v]) => (
                  <div key={k}>
                    <dt>{k.replaceAll("_", " ")}</dt>
                    <dd>{String(v) || "Default"}</dd>
                  </div>
                ))}
              </dl>
              {can(a.session, "admin:ConfigUpdate") && (
                <Button
                  variant="outline"
                  onClick={() =>
                    a.edit({
                      title: `Edit ${subsystem} configuration`,
                      description:
                        "Change operational settings. Review the current values before saving. A restart may be required.",
                      fields: Object.entries(config.data?.values || {}).map(
                        ([name, value]) => ({
                          name,
                          label: name.replaceAll("_", " "),
                          value: String(value),
                        }),
                      ),
                      run: async (values) => {
                        const result = await api(
                          `/minigo/config/${enc(subsystem)}`,
                          "PUT",
                          { values },
                        );
                        setRestart(result.restartRequired === true);
                      },
                      onSuccess: () => setRev((x) => x + 1),
                    })
                  }
                >
                  <Settings2 size={16} />
                  Edit settings
                </Button>
              )}
            </div>
          )
        )}
      </section>
    </>
  );
}
