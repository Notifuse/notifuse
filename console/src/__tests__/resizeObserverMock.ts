// jsdom implements no ResizeObserver. antd v6 mounts one in far more places
// than v5 did — Typography with `ellipsis`, and any Table that scrolls
// horizontally — so rendering those pages throws before a single assertion
// runs. The stub never reports a size: these tests assert content, not layout.
class ResizeObserverStub implements ResizeObserver {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = ResizeObserverStub
}
