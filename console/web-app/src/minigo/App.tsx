import { useEffect, useState, type FormEvent } from "react";
import {
  ArrowRight,
  Boxes,
  Database,
  ExternalLink,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  Server,
  Shield,
  Sun,
  UsersRound,
  UserRound,
} from "lucide-react";
import { api, base, can, setAnonymous, type Data } from "./api";
import {
  Button,
  Input,
  ErrorBox,
  Loading,
  Notice,
  EditorDialog,
  type Editor,
} from "./shared";
import { Overview, Buckets, ObjectBrowser } from "./Storage";
import { BucketSettings, Identity, System } from "./Admin";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "./components/ui/dialog";
function getPath() {
  return (
    "/" +
    location.pathname.slice(base.length).replace(/^\//, "") +
    location.search
  );
}
function Logo() {
  return (
    <div className="brand">
      <div className="brand-symbol">
        <Boxes size={23} strokeWidth={1.7} />
      </div>
      <span>
        minigo<span className="brand-dot">.</span>
      </span>
    </div>
  );
}
function Login({ ready }: { ready: (s: Data) => void }) {
  const [name, setName] = useState(""),
    [password, setPassword] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [details, setDetails] = useState<Data | null>(null);
  useEffect(() => {
    api("/login")
      .then(setDetails)
      .catch((e) => setError(e.message));
  }, []);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/login", "POST", { accessKey: name, secretKey: password });
      setPassword("");
      ready(await api("/session"));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="login-page">
      <div className="login-story">
        <Logo />
        <div>
          <div className="eyebrow">YOUR STORAGE. YOUR CONTROL.</div>
          <h1>
            Built for your
            <br />
            next big thing.
          </h1>
          <p>
            One clear view of your objects, your team,
            <br />
            and the infrastructure behind it all.
          </p>
          <div className="login-illustration" aria-hidden="true">
            <div className="storage-block">
              <Boxes size={50} />
              <span>OBJECT STORAGE</span>
            </div>
            <div className="storage-line" />
            <div className="storage-chips">
              <span>S3 compatible</span>
              <span>Go powered</span>
            </div>
          </div>
        </div>
        <span className="story-footer">
          MINIGO · OPEN SOURCE OBJECT STORAGE
        </span>
      </div>
      <main className="login-main">
        <div className="mobile-brand">
          <Logo />
        </div>
        <div className="login-form">
          <div className="eyebrow">MINIGO CONSOLE</div>
          <h2>Welcome back</h2>
          <p>Sign in to manage your storage.</p>
          <ErrorBox message={error} />
          {!details && !error ? (
            <Loading />
          ) : (
            <>
              <form onSubmit={submit}>
                <div className="field">
                  <label htmlFor="access-key">Username or access key</label>
                  <Input
                    id="access-key"
                    name="accessKey"
                    value={name}
                    autoComplete="username"
                    autoFocus
                    required
                    disabled={busy}
                    placeholder="Enter your username"
                    onChange={(e) => setName(e.target.value)}
                  />
                </div>
                <div className="field">
                  <label htmlFor="password">Password or secret key</label>
                  <Input
                    id="password"
                    name="secretKey"
                    type="password"
                    autoComplete="current-password"
                    value={password}
                    required
                    disabled={busy}
                    placeholder="Enter your password"
                    onChange={(e) => setPassword(e.target.value)}
                  />
                </div>
                <Button
                  className="w-full"
                  type="submit"
                  disabled={busy || !name || !password}
                >
                  {busy ? "Signing in…" : "Sign in"}
                  <ArrowRight size={17} />
                </Button>
              </form>
              {(details?.redirectRules || []).length > 0 && (
                <div className="sso">
                  <span>or continue with</span>
                  {details!.redirectRules.map((r: Data, i: number) => (
                    <Button asChild variant="outline" key={i}>
                      <a href={r.redirect}>
                        {r.displayName || r.serviceType || "Single sign-on"}
                        <ExternalLink size={14} />
                      </a>
                    </Button>
                  ))}
                </div>
              )}
            </>
          )}
          <p className="login-note">
            <Shield size={15} />
            Your session is protected with server-managed cookies.
          </p>
        </div>
        <footer>
          Minigo Console <span>Built on MinIO · AGPLv3</span>
        </footer>
      </main>
    </div>
  );
}
export default function App() {
  const [session, setSession] = useState<Data | null>(null),
    [loading, setLoading] = useState(true),
    [initialError, setInitialError] = useState(""),
    [path, setPath] = useState(getPath),
    [editor, setEditor] = useState<Editor | null>(null),
    [notice, setNotice] = useState(""),
    [mobile, setMobile] = useState(false),
    [dark, setDark] = useState(() => {
      try {
        const value = localStorage.getItem("minigo-theme");
        return value
          ? value === "dark"
          : matchMedia("(prefers-color-scheme: dark)").matches;
      } catch {
        return false;
      }
    });
  function navigate(to: string) {
    history.pushState(null, "", base + to.replace(/^\//, ""));
    setPath(to);
    setMobile(false);
    window.scrollTo(0, 0);
  }
  useEffect(() => {
    const pop = () => setPath(getPath()),
      expired = () => {
        setSession(null);
        setEditor(null);
        setNotice("Your session expired. Please sign in again.");
      };
    window.addEventListener("popstate", pop);
    window.addEventListener("session-expired", expired);
    const initialize = async () => {
      try {
        if (path === "/logout") {
          await api("/logout", "POST", {});
          navigate("/login");
          return;
        }
        setSession(await api("/session"));
      } catch (e) {
        const parts = path.split("?")[0].split("/");
        if (parts[1] === "browser" && parts[2]) {
          try {
            setAnonymous(true);
            await api(
              `/buckets/${encodeURIComponent(decodeURIComponent(parts[2]))}/objects`,
              "GET",
              undefined,
              { limit: 1 },
            );
            setSession({ anonymous: true });
          } catch {
            setAnonymous(false);
          }
        } else if ((e as any).status !== 401 && (e as any).status !== 403) {
          setInitialError((e as Error).message);
        }
      } finally {
        setLoading(false);
      }
    };
    void initialize();
    return () => {
      window.removeEventListener("popstate", pop);
      window.removeEventListener("session-expired", expired);
    };
  }, []);
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    try {
      localStorage.setItem("minigo-theme", dark ? "dark" : "light");
    } catch {}
  }, [dark]);
  async function logout() {
    try {
      await api("/logout", "POST", {});
      setSession(null);
      setAnonymous(false);
      navigate("/login");
    } catch (e) {
      setNotice((e as Error).message);
    }
  }
  if (loading)
    return (
      <div className="app-loading">
        <Logo />
        <Loading />
      </div>
    );
  if (!session)
    return (
      <>
        <Login
          ready={(s) => {
            setAnonymous(false);
            setSession(s);
            setInitialError("");
            if (path === "/login" || path === "/logout" || path === "/")
              navigate("/overview");
          }}
        />
        {initialError && (
          <div className="connection-error">
            <ErrorBox message={initialError} />
          </div>
        )}
        <Notice message={notice} onClose={() => setNotice("")} />
      </>
    );
  const sections: {
    title: string;
    items: {
      path: string;
      label: string;
      icon: typeof Database;
      permission?: string;
    }[];
  }[] = [
    {
      title: "WORKSPACE",
      items: [
        { path: "/overview", label: "Overview", icon: LayoutDashboard },
        { path: "/buckets", label: "Buckets", icon: Database },
      ],
    },
    {
      title: "ACCESS MANAGEMENT",
      items: [
        {
          path: "/identity/users",
          label: "Users",
          icon: UserRound,
          permission: "admin:ListUsers",
        },
        {
          path: "/identity/groups",
          label: "Groups",
          icon: UsersRound,
          permission: "admin:ListGroups",
        },
        {
          path: "/policies",
          label: "Policies",
          icon: Shield,
          permission: "admin:ListUserPolicies",
        },
        {
          path: "/access-keys",
          label: "Access keys",
          icon: KeyRound,
        },
      ],
    },
    {
      title: "SYSTEM",
      items: [
        {
          path: "/system",
          label: "Deployment",
          icon: Server,
          permission: "admin:ServerInfo",
        },
      ],
    },
  ];
  const active = (p: string) =>
    p === "/buckets"
      ? path.startsWith("/browser") || path.startsWith("/buckets")
      : path === p || (p === "/overview" && path === "/");
  const sidebar = (
    <>
      <Logo />
      <div className="workspace-select">
        <div className="workspace-avatar">M</div>
        <div>
          <strong>Storage workspace</strong>
          <small>
            {session.anonymous ? "Public access" : "Object storage"}
          </small>
        </div>
        <span className="connection-dot" />
      </div>
      <nav aria-label="Main navigation">
        {sections.map((s) => {
          const items = s.items.filter(
            (i) =>
              (i.path !== "/access-keys" || !session.anonymous) &&
              (!i.permission || can(session, i.permission)),
          );
          return (
            !!items.length && (
              <div className="nav-group" key={s.title}>
                <h2>{s.title}</h2>
                {items.map((i) => (
                  <button
                    key={i.path}
                    className={active(i.path) ? "active" : ""}
                    aria-current={active(i.path) ? "page" : undefined}
                    onClick={() => navigate(i.path)}
                  >
                    <i.icon size={18} />
                    {i.label}
                  </button>
                ))}
              </div>
            )
          );
        })}
      </nav>
      <div className="sidebar-bottom">
        <div className="sidebar-note">
          <Shield size={17} />
          <span>
            S3 compatible.
            <br />
            <strong>Built on a proven foundation.</strong>
          </span>
        </div>
        <div className="account">
          <div className="account-avatar">
            <UserRound size={17} />
          </div>
          <div>
            <strong>
              {session.anonymous ? "Anonymous" : "Authenticated session"}
            </strong>
            <small>
              {session.anonymous
                ? "Public objects only"
                : "Server-managed access"}
            </small>
          </div>
          <button aria-label="Sign out" onClick={logout}>
            <LogOut size={17} />
          </button>
        </div>
      </div>
    </>
  );
  const actions = { edit: setEditor, notify: setNotice, session, navigate };
  const segments = path.split("?")[0].split("/").filter(Boolean);
  let page;
  if (segments[0] === "browser" && segments[1])
    page = (
      <ObjectBrowser
        key={segments[1]}
        {...actions}
        bucket={decodeURIComponent(segments[1])}
        prefix={
          new URLSearchParams(path.split("?")[1]).get("prefix") ??
          segments.slice(2).map(decodeURIComponent).join("/") +
            (path.endsWith("/") && segments.length > 2 ? "/" : "")
        }
      />
    );
  else if (
    segments[0] === "buckets" &&
    segments[1] &&
    segments[2] === "settings"
  )
    page = (
      <BucketSettings {...actions} bucket={decodeURIComponent(segments[1])} />
    );
  else if (path === "/buckets" || path === "/browser")
    page = <Buckets {...actions} />;
  else if (path === "/identity/users" || path === "/identity/groups")
    page = (
      <Identity
        key={path}
        {...actions}
        kind={segments[1] as "users" | "groups"}
      />
    );
  else if (path === "/policies" || path === "/access-keys")
    page = (
      <Identity
        key={path}
        {...actions}
        kind={path === "/policies" ? "policies" : "service-accounts"}
      />
    );
  else if (path === "/system") page = <System {...actions} />;
  else page = <Overview {...actions} />;
  return (
    <div className="app-shell">
      <aside className="sidebar">{sidebar}</aside>
      <Dialog open={mobile} onOpenChange={setMobile}>
        <DialogContent className="mobile-nav">
          <DialogHeader className="sr-only">
            <DialogTitle>Navigation</DialogTitle>
            <DialogDescription>Browse the Minigo console.</DialogDescription>
          </DialogHeader>
          {sidebar}
        </DialogContent>
      </Dialog>
      <div className="workspace">
        <header className="topbar">
          <div className="topbar-left">
            <Button
              variant="ghost"
              size="icon"
              className="mobile-toggle"
              aria-label="Open navigation"
              onClick={() => setMobile(true)}
            >
              <Menu size={20} />
            </Button>
            <span>Console</span>
            <span className="topbar-slash">/</span>
            <strong>
              {path.startsWith("/browser")
                ? "Object browser"
                : path.includes("settings")
                  ? "Bucket settings"
                  : sections.flatMap((s) => s.items).find((i) => active(i.path))
                      ?.label || "Overview"}
            </strong>
          </div>
          <div className="topbar-right">
            <span className="session-badge">
              <span className="connection-dot" />
              {session.anonymous ? "Public session" : "Connected"}
            </span>
            <Button
              size="icon"
              variant="ghost"
              aria-label={
                dark ? "Switch to light theme" : "Switch to dark theme"
              }
              onClick={() => setDark(!dark)}
            >
              {dark ? <Sun size={18} /> : <Moon size={18} />}
            </Button>
          </div>
        </header>
        <main className="main-content">
          {page}
          <footer className="page-footer">
            <span>MINIGO CONSOLE</span>
            <span>Open source object storage · AGPLv3</span>
          </footer>
        </main>
      </div>
      <EditorDialog
        editor={editor}
        onClose={() => setEditor(null)}
        notify={setNotice}
      />
      <Notice message={notice} onClose={() => setNotice("")} />
    </div>
  );
}
