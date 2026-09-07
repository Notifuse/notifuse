import { router } from "../../router";

// Defined in ./errors so it can be imported without pulling in the router; re-exported
// here because most call sites already import it from this module.
export { ApiError } from "./errors";
import {
  ApiError,
  permissionDeniedMessage,
  permissionDenialFromBody,
} from "./errors";
import { licenseRefusalFromBody, licenseRefusedMessage } from "./licenseErrors";

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const errorData = await response.json().catch(() => null);

    if (
      response.status === 401 ||
      errorData?.error === "Session expired" ||
      errorData?.message === "Session expired"
    ) {
      localStorage.removeItem("auth_token");

      // A 401 also lands while the sign-in page itself is booting: a stale token in
      // localStorage makes AuthContext's opening user.me call fail. Navigating from
      // there rewrites the address bar to a bare /console/signin, and since the
      // router does not carry search params across a navigate, it drops the query
      // string the page still needs — ?email= is what drives the one-click sign-in
      // link. The visitor lands on an empty form, and only the next attempt works,
      // because this handler has meanwhile cleared the token. Already being on the
      // sign-in route means there is nowhere to send them anyway.
      if (window.location.pathname !== "/console/signin") {
        router.navigate({ to: "/console/signin" });
      }
    }

    // A permission denial carries the missing grant alongside its message. Swapping in a
    // translated sentence here is what gets it in front of the user: roughly a third of the
    // console's error handlers render `error.message` verbatim, and none of them would
    // otherwise show anything but the server's English prose. errorData is passed through
    // untouched, so a handler that wants the resource and verb still reads them off `data`.
    const denial = permissionDenialFromBody(errorData);

    // A licence refusal gets the same treatment for the same reason: the console names the
    // capability and the remedy, rather than passing the server's English prose through.
    const refusal = denial ? null : licenseRefusalFromBody(errorData);

    const message = denial
      ? permissionDeniedMessage(denial)
      : refusal
        ? licenseRefusedMessage(refusal)
        : errorData?.error;

    throw new ApiError(
      message || "An error occurred",
      response.status,
      errorData,
    );
  }
  return response.json();
}

// A non-2xx answer is turned into the same ApiError a JSON request would throw,
// so a refused download reads exactly like a refused request.
async function throwIfNotOk(response: Response): Promise<void> {
  if (response.ok) return;
  await handleResponse<never>(response);
}

async function fetchWithAuth(
  endpoint: string,
  options: RequestInit = {},
): Promise<Response> {
  const authToken = localStorage.getItem("auth_token");
  const headers = {
    "Content-Type": "application/json",
    ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
    ...options.headers,
  };
  let defaultOrigin = window.location.origin;
  if (defaultOrigin.includes("notifusedev.com")) {
    defaultOrigin = "https://localapi.notifuse.com:4000";
  }
  const apiEndpoint =
    window.API_ENDPOINT?.trim().replace(/\/+$/, "") || defaultOrigin;
  return fetch(`${apiEndpoint}${endpoint}`, {
    ...options,
    headers,
  });
}

async function request<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetchWithAuth(endpoint, options);
  return handleResponse<T>(response);
}

export const api = {
  get: <T>(endpoint: string) => request<T>(endpoint),
  post: <T>(endpoint: string, data: unknown) =>
    request<T>(endpoint, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  put: <T>(endpoint: string, data: unknown) =>
    request<T>(endpoint, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  delete: <T>(endpoint: string) =>
    request<T>(endpoint, {
      method: "DELETE",
    }),
  // POSTs a JSON body and returns the raw response body as a Blob — for the
  // endpoints that stream a file rather than answer JSON.
  download: async (endpoint: string, data: unknown): Promise<Blob> => {
    const response = await fetchWithAuth(endpoint, {
      method: "POST",
      body: JSON.stringify(data),
    });
    await throwIfNotOk(response);
    return response.blob();
  },
};
