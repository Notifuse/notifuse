/**
 * The blog theme preview runs in an origin of its own, where reading storage or
 * cookies throws and stops the theme script that tried. This script gives it
 * in-memory ones instead, which last as long as the preview does. Items can be
 * read and written as properties too (`localStorage.theme`), as on a real
 * Storage. A cookie set to expire now or earlier is removed; its other
 * attributes are ignored. IndexedDB and the Cache API stay unavailable there.
 */
export const PREVIEW_STORAGE_SCRIPT = `<script>(function () {
  function memoryStorage() {
    var items = new Map();
    var methods = {
      key: function (i) { var key = Array.from(items.keys())[i >>> 0]; return key === undefined ? null : key; },
      getItem: function (k) { k = String(k); return items.has(k) ? items.get(k) : null; },
      setItem: function (k, v) { items.set(String(k), String(v)); },
      removeItem: function (k) { items.delete(String(k)); },
      clear: function () { items.clear(); }
    };
    return new Proxy(Object.create(methods), {
      get: function (target, name) {
        if (name === 'length') return items.size;
        if (typeof name !== 'string' || name in target) return target[name];
        return items.has(name) ? items.get(name) : undefined;
      },
      set: function (target, name, value) { items.set(String(name), String(value)); return true; },
      has: function (target, name) { return name === 'length' || name in target || items.has(name); },
      deleteProperty: function (target, name) { items.delete(String(name)); return true; },
      ownKeys: function () { return Array.from(items.keys()); },
      getOwnPropertyDescriptor: function (target, name) {
        if (!items.has(name)) return undefined;
        return { value: items.get(name), writable: true, enumerable: true, configurable: true };
      }
    });
  }
  function define(target, name, descriptor) {
    try { Object.defineProperty(target, name, Object.assign({ configurable: true }, descriptor)); } catch (e) {}
  }
  var cookies = new Map();
  define(window, 'localStorage', { value: memoryStorage() });
  define(window, 'sessionStorage', { value: memoryStorage() });
  define(document, 'cookie', {
    get: function () {
      return Array.from(cookies, function (entry) { return entry[0] + '=' + entry[1]; }).join('; ');
    },
    set: function (value) {
      var parts = String(value).split(';');
      var at = parts[0].indexOf('=');
      if (at <= 0) return;
      var name = parts[0].slice(0, at).trim();
      var maxAge = null;
      var expires = null;
      parts.slice(1).forEach(function (part) {
        var eq = part.indexOf('=');
        var key = (eq < 0 ? part : part.slice(0, eq)).trim().toLowerCase();
        var val = eq < 0 ? '' : part.slice(eq + 1).trim();
        if (key === 'max-age' && /^-?\\d+$/.test(val)) maxAge = Number(val);
        if (key === 'expires' && !isNaN(Date.parse(val))) expires = Date.parse(val);
      });
      // Max-Age wins over Expires, as in a browser
      var expired = maxAge !== null ? maxAge <= 0 : expires !== null && expires <= Date.now();
      if (expired) cookies.delete(name);
      else cookies.set(name, parts[0].slice(at + 1).trim());
    }
  });
})();</script>`

/**
 * The preview document with that script first, so it runs before anything the
 * theme does: right after the doctype, so the document still has one.
 */
export function withPreviewStorage(html: string): string {
  const doctype = /^\s*<!doctype[^>]*>/i.exec(html)?.[0] ?? ''
  return doctype + PREVIEW_STORAGE_SCRIPT + html.slice(doctype.length)
}
