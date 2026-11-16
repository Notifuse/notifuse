import { Tooltip } from "antd";
import { InfoCircleOutlined } from "@ant-design/icons";
import { i18n } from "@lingui/core";
import { useLingui } from "@lingui/react/macro";
import { KpiStrip, type KpiItem } from "../../layout";
import { useWebAnalytics } from "../useWebAnalytics";
import { Delta } from "../Delta";
import { deltaTone } from "../lib/delta";
import { ExploreTotals } from "../lib/exploreRows";
import { formatDimensionValue, getDimensionLabel } from "../lib/dimensions";
import { formatValue } from "../lib/format";
import { getHeatMapStyle } from "../lib/heatmap";
import { SESSION_METRICS, TIMESCORE_REFERENCE_SECONDS } from "../lib/types";

interface ExploreSummaryProps {
  totals?: ExploreTotals;
  showComparison: boolean;
  loading?: boolean;
  /** Highest TimeScore across every combination, which scales the heat dot. */
  bestValue: number;
  /** TimeScore of the best-performing combination in the whole report. */
  bestTimeScore?: number;
  /** The winning row, read for the dimension values that achieved it. */
  bestCombination?: Record<string, unknown>;
  /** The combination query failed; the tile says so instead of vanishing. */
  bestError?: boolean;
  /** Report dimensions, in drill-down order — the order the tooltip lists. */
  dimensions: string[];
}

/**
 * Period totals for the whole report, above the drill-down.
 *
 * These come from an ungrouped query rather than a sum of the visible rows: a
 * median or a rate cannot be recovered by aggregating the medians and rates of
 * its parts.
 */
export function ExploreSummary(props: ExploreSummaryProps) {
  const { t } = useLingui();
  const context = useWebAnalytics();

  // Hidden rather than shown as zero: before the query lands there is no
  // honest value, and a report with no engaged time has no winner to name.
  const hasBest = props.bestTimeScore !== undefined && props.bestTimeScore > 0;
  // A failure still occupies the slot. Hiding it would be indistinguishable
  // from "this report has no engaged time", and this is the heaviest query on
  // the page — the one most likely to be the thing that failed.
  const showBest = hasBest || Boolean(props.bestError);

  const labels: Record<string, string> = {
    sessions: t`Sessions`,
    median_duration: t`Median TimeScore`,
    bounce_rate: t`Bounce Rate`,
    median_scroll: t`Median Scroll Depth`,
  };
  const tooltips: Record<string, string> = {
    median_duration: t`TimeScore is the median engaged time across all sessions`,
  };

  const bestHint = hasBest ? (
    <div className="text-xs">
      <div className="mb-1 font-medium">{t`Best performing combination:`}</div>
      {props.dimensions.map((dimension) => (
        <div key={dimension}>
          <span className="opacity-70">
            {getDimensionLabel(dimension, context.customDimensionLabels)}:
          </span>{" "}
          <span className="font-medium">
            {formatDimensionValue(
              dimension,
              props.bestCombination?.[dimension],
              {
                emptyLabel: t`(empty)`,
                locale: i18n.locale,
              },
            )}
          </span>
        </div>
      ))}
      {/* The tiles around it deliberately ignore the threshold and the
          metric filters; this one cannot, or it would name a
          one-session combination as the best. Say which it is. */}
      {context.minSessions > 1 ? (
        <div className="mt-1 opacity-70">
          {t`Among combinations with at least ${context.minSessions} sessions.`}
        </div>
      ) : null}
    </div>
  ) : (
    t`The best-performing combination could not be loaded.`
  );

  const bestItem: KpiItem = {
    key: "best",
    label: t`Best TimeScore`,
    value: hasBest ? (
      <span className="inline-flex cursor-help items-center gap-2">
        <span
          style={getHeatMapStyle(
            props.bestTimeScore ?? 0,
            props.bestValue,
            TIMESCORE_REFERENCE_SECONDS,
            10,
          )}
        />
        {formatValue(props.bestTimeScore ?? 0, "duration")}
      </span>
    ) : (
      <span className="cursor-help text-ink-4">—</span>
    ),
    hint: bestHint,
  };

  // The best combination belongs next to the median it should be read against
  // — one is what a typical session managed, the other what the best segment
  // did — rather than stranded at the end of the row.
  const items: KpiItem[] = [];
  for (const metric of SESSION_METRICS) {
    const value = props.totals?.[metric.key as keyof ExploreTotals] as
      | number
      | undefined;
    const change = props.showComparison
      ? (props.totals?.[`${metric.key}_change` as keyof ExploreTotals] as
          | number
          | undefined)
      : undefined;
    const tooltip = tooltips[metric.key];
    const tone = deltaTone(change, metric.invertTrend);

    items.push({
      key: metric.key,
      label: (
        <span className="inline-flex items-center gap-1">
          {labels[metric.key] ?? metric.label}
          {tooltip ? (
            <Tooltip title={tooltip}>
              <InfoCircleOutlined className="text-overline" />
            </Tooltip>
          ) : null}
        </span>
      ),
      value: (
        <span className="inline-flex items-center gap-2">
          {metric.key === "median_duration" ? (
            <span
              style={getHeatMapStyle(
                value ?? 0,
                props.bestValue,
                TIMESCORE_REFERENCE_SECONDS,
                10,
              )}
            />
          ) : null}
          {formatValue(value ?? 0, metric.format)}
        </span>
      ),
      // Nothing to print for a zero or missing change: no empty line under the value.
      sub:
        tone === "neutral" ? undefined : (
          <Delta
            change={change}
            invertTrend={metric.invertTrend}
            decimals={1}
          />
        ),
      tone,
    });
    if (showBest && metric.key === "median_duration") items.push(bestItem);
  }

  return <KpiStrip items={items} loading={props.loading && !props.totals} />;
}
