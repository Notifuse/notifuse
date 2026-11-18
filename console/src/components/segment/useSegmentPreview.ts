import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { debounce } from 'lodash'
import { useLingui } from '@lingui/react/macro'
import {
  previewSegment,
  type PreviewSegmentRequest,
  type PreviewSegmentResponse,
  type SegmentValueErrorDetails,
  type TreeNode
} from '../../services/api/segment'
import { HasLeaf, isTreeQueryable } from './tree_completeness'
import { readSegmentValueError } from './segmentValueError'

const PREVIEW_LIMIT = 100
// Long enough that a burst of typing inside a condition costs a single count query.
const PREVIEW_DEBOUNCE_MS = 600

/**
 * The count of the segment as the form stands: the committed tree, overridden by the condition
 * open in its editor (the draft), counted with a debounce and only when it compiles.
 */
export function useSegmentPreview(workspaceId: string | undefined, committedTree: TreeNode | undefined) {
  const { t } = useLingui()
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [previewResponse, setPreviewResponse] = useState<PreviewSegmentResponse | undefined>()
  const [previewedHash, setPreviewedHash] = useState<string | undefined>() // request behind previewResponse
  const [previewError, setPreviewErrorState] = useState<string | undefined>()
  // Set with previewError when the count failed on a contact value the conditions cannot convert:
  // retrying cannot help until that value or the conditions change.
  const [valueError, setValueError] = useState<SegmentValueErrorDetails | undefined>()
  const setPreviewError = useCallback((message: string | undefined, details?: SegmentValueErrorDetails) => {
    setPreviewErrorState(message)
    setValueError(details)
  }, [])
  // Set while a condition is open in its form, so the count can follow it before it is confirmed
  const [draftTree, setDraftTreeState] = useState<TreeNode | undefined>()
  // False while the open condition breaks a rule of its form, which Confirm would refuse
  const [draftCountable, setDraftCountable] = useState(true)
  const setDraftTree = useCallback((tree: TreeNode | undefined, countable = true) => {
    setDraftTreeState(tree)
    setDraftCountable(countable)
  }, [])

  // A condition only reaches the tree on Confirm, so without the draft the count would ignore
  // what is on screen.
  const effectiveTree = draftTree ?? committedTree

  // Only trees the backend can compile are worth a request; a half-filled condition keeps the
  // last count on screen instead of replacing it with an error.
  const previewHash = useMemo(() => {
    if (!workspaceId || !effectiveTree || !isTreeQueryable(effectiveTree)) return undefined
    if (draftTree && !draftCountable) return undefined

    const requestData: PreviewSegmentRequest = {
      workspace_id: workspaceId,
      tree: effectiveTree,
      limit: PREVIEW_LIMIT
    }
    return JSON.stringify(requestData)
  }, [workspaceId, effectiveTree, draftTree, draftCountable])

  const requestSeqRef = useRef(0)
  // The request queued in the debounce or already in flight — what an answer is on its way for,
  // as opposed to previewedHash, which is the answer currently on screen.
  const pendingHashRef = useRef<string | undefined>(undefined)
  const hasRequestedRef = useRef(false)

  const runPreview = useCallback(
    async (tree: TreeNode, hash: string) => {
      if (!workspaceId) return

      const seq = ++requestSeqRef.current
      hasRequestedRef.current = true
      setLoadingPreview(true)

      try {
        const res = await previewSegment({
          workspace_id: workspaceId,
          tree: tree,
          limit: PREVIEW_LIMIT
        })
        // A newer edit was requested in the meantime, that answer is the one that counts
        if (seq !== requestSeqRef.current) return

        setPreviewResponse(res)
        setPreviewedHash(hash)
        setPreviewError(undefined)
      } catch (error) {
        if (seq !== requestSeqRef.current) return

        // No toast here: this runs on every edit, so the failure is reported on the circle itself
        console.error('Preview error:', error)
        setPreviewError(
          error instanceof Error ? error.message : t`Failed to preview segment`,
          readSegmentValueError(error)
        )
      } finally {
        if (seq === requestSeqRef.current) {
          // Only release the marker this run owns; a newer edit may already be queued behind it
          if (pendingHashRef.current === hash) pendingHashRef.current = undefined
          setLoadingPreview(false)
        }
      }
    },
    [workspaceId, t, setPreviewError]
  )

  // Held in a ref so the debounce survives re-renders: rebuilding it would cancel the call
  // pending for the edit in progress, and typing re-renders constantly.
  const runPreviewRef = useRef(runPreview)
  runPreviewRef.current = runPreview

  const debouncedPreview = useMemo(
    () =>
      debounce(
        (tree: TreeNode, hash: string) => runPreviewRef.current(tree, hash),
        PREVIEW_DEBOUNCE_MS
      ),
    []
  )

  useEffect(() => () => debouncedPreview.cancel(), [debouncedPreview])

  // Refresh whenever the tree — committed or in progress — settles on something not counted yet
  useEffect(() => {
    if (!previewHash || !effectiveTree) return

    if (previewHash === previewedHash) {
      // The form is back on the count already displayed. Drop what is queued or in flight for the
      // states in between: their answers would replace a count we know matches the form.
      debouncedPreview.cancel()
      requestSeqRef.current++
      pendingHashRef.current = undefined
      setLoadingPreview(false)
      // Any error describes one of those abandoned states, not the count now on screen
      setPreviewError(undefined)
      return
    }

    if (previewHash === pendingHashRef.current) return

    pendingHashRef.current = previewHash
    setPreviewError(undefined)

    // The first count, i.e. opening an existing segment, has nothing to debounce against
    if (!hasRequestedRef.current) {
      runPreviewRef.current(effectiveTree, previewHash)
      return
    }

    debouncedPreview(effectiveTree, previewHash)
    // runPreview is deliberately absent: it changes identity on every render (its `t` dependency
    // does), which would re-run this on every render for no reason.
  }, [previewHash, previewedHash, effectiveTree, debouncedPreview, setPreviewError])

  const previewNow = () => {
    if (!previewHash || !effectiveTree) return

    debouncedPreview.cancel()
    pendingHashRef.current = previewHash
    runPreview(effectiveTree, previewHash)
  }

  const hasPreview = previewResponse !== undefined
  // The count on screen no longer answers what the form says: either a request is on its way, or
  // the condition being edited is not complete enough to ask for one yet.
  const isPreviewStale = hasPreview && previewedHash !== previewHash
  // A count is on its way: either the request is out, or it is waiting out the debounce. Both are
  // the same wait as far as the user is concerned, so both get the spinner.
  const isPreviewRefreshing =
    loadingPreview || (!!previewHash && previewHash !== previewedHash && !previewError)
  // Stale with nothing on its way: the form as it stands cannot be counted, so the number is
  // frozen until the condition is finished. A refresh merely waiting its turn is not this.
  const isPreviewBlocked =
    isPreviewStale &&
    !previewHash &&
    !loadingPreview &&
    !previewError &&
    !!effectiveTree &&
    HasLeaf(effectiveTree)

  return {
    effectiveTree,
    setDraftTree,
    previewResponse,
    previewHash,
    previewError,
    valueError,
    isPreviewStale,
    isPreviewRefreshing,
    isPreviewBlocked,
    previewNow
  }
}
