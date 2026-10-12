export type Data = Record<string, any>;
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code = "",
  ) {
    super(message);
  }
}
export const base = new URL(document.baseURI).pathname.replace(/\/?$/, "/");
export function url(path: string, query: Data = {}) {
  const u = new URL(`${base}api/v1${path}`, window.location.origin);
  Object.entries(query).forEach(([k, v]) => {
    if (v !== undefined && v !== null) u.searchParams.set(k, String(v));
  });
  return u.toString();
}
let anonymous = false;
export function setAnonymous(value: boolean) {
  anonymous = value;
}
export async function api<T = Data>(
  path: string,
  method = "GET",
  body?: unknown,
  query?: Data,
): Promise<T> {
  const headers: Record<string, string> = {};
  if (method !== "GET" && !(body instanceof FormData))
    headers["Content-Type"] = "application/json";
  if (anonymous) headers["X-Anonymous"] = "1";
  const res = await fetch(url(path, query), {
    method,
    credentials: "same-origin",
    cache: "no-store",
    headers,
    body:
      body === undefined
        ? undefined
        : body instanceof FormData
          ? body
          : JSON.stringify(body),
  });
  const text = await res.text();
  let data: any;
  try {
    data = text ? JSON.parse(text) : {};
  } catch {
    data = text;
  }
  if (!res.ok) {
    if (
      (res.status === 401 || data?.message === "invalid session") &&
      path !== "/session" &&
      path !== "/login"
    )
      window.dispatchEvent(new Event("session-expired"));
    throw new ApiError(
      data?.detailedMessage ||
        data?.message ||
        `Request failed (${res.status})`,
      res.status,
      data?.code || "",
    );
  }
  return data as T;
}
export function can(session: Data, action: string, resource = "*") {
  if (session.anonymous)
    return ["s3:GetObject", "s3:ListBucket", "s3:PutObject"].includes(action);
  const target = resource.startsWith("arn:")
    ? resource
    : `arn:aws:s3:::${resource}`;
  const glob = (pattern: string, value: string) =>
    new RegExp(
      "^" +
        pattern
          .replace(/[.+^${}()|[\]\\]/g, "\\$&")
          .replace(/\*/g, ".*")
          .replace(/\?/g, ".") +
        "$",
    ).test(value);
  return Object.entries(session.permissions || {}).some(
    ([name, grants]) =>
      (resource === "*" ||
        (name === "console-ui" && action.startsWith("admin:")) ||
        glob(name, target) ||
        glob(name, `${target}/*`)) &&
      (grants as string[]).some((x) => x === "*" || glob(x, action)),
  );
}
export function bytes(n?: number) {
  if (n === undefined || n === null) return "—";
  if (!n) return "0 B";
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), 5);
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: i ? 1 : 0 }).format(n / 1024 ** i)} ${["B", "KB", "MB", "GB", "TB", "PB"][i]}`;
}
export function date(value?: string) {
  return value
    ? new Date(value).toLocaleString(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      })
    : "—";
}
export const enc = encodeURIComponent;
