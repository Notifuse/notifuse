import { useLingui } from '@lingui/react/macro'
import type { DraftIssue } from './validateDraft'

/** The words of a draft issue: "variant B has no template", shown after "Incomplete:" and in the summary. */
export function useIssueText() {
  const { t } = useLingui()
  return (issue: DraftIssue): string => {
    switch (issue.code) {
      case 'name_missing':
        return t`the broadcast has no name`
      case 'name_too_long':
        return t`the name is longer than 255 characters`
      case 'list_missing':
        return t`choose a list`
      case 'ab_too_few':
        return t`an A/B test needs at least 2 variants`
      case 'ab_too_many':
        return t`an A/B test has 5 variants at most`
      case 'variant_no_template': {
        const variant = issue.variant
        return t`variant ${variant} has no template`
      }
      case 'template_reused': {
        const variant = issue.variant
        return t`variant ${variant} uses a template already used in another variant`
      }
      case 'sender_gone': {
        const variant = issue.variant
        return t`the sender of variant ${variant} no longer exists`
      }
      case 'sample_out_of_range':
        return t`the test sample is between 1 and 100 %`
      case 'duration_out_of_range':
        return t`the test lasts between 1 and 168 hours`
      case 'metric_missing':
        return t`choose the metric that picks the winner`
      case 'auto_winner_untracked':
        return t`the winner can't be sent automatically without tracking`
      case 'feed_url_missing':
        return issue.feed === 'global' ? t`the global data feed has no URL` : t`the per-recipient data feed has no URL`
      case 'feed_url_invalid':
        return issue.feed === 'global'
          ? t`the global data feed URL must start with https:// or http://, without query or fragment`
          : t`the per-recipient data feed URL must start with https:// or http://, without query or fragment`
      case 'feed_header_incomplete':
        return issue.feed === 'global'
          ? t`a header of the global data feed has no name or value`
          : t`a header of the per-recipient data feed has no name or value`
    }
  }
}
