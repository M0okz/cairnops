# shadcn-svelte chart primitives

Copied from [shadcn-svelte](https://github.com/huntabyte/shadcn-svelte/tree/3313e3eeb9b703da6bf8701c61c25a845e975aa3/docs/src/lib/registry/ui/chart), commit `3313e3eeb9b703da6bf8701c61c25a845e975aa3`. MIT license included. This is the official `Chart.Tooltip` implementation, using LayerChart's tooltip primitive and context.

Local integration changes in `chart-tooltip.svelte`:

- `valueFormatter` preserves localized units and boolean values without replacing the official row layout.
- `rootProps` exposes LayerChart positioning and motion options. CairnOps keeps the tooltip inside the chart, including inside a native incident dialog, and respects reduced motion.
- Indicator colors use Svelte `style:` directives (CSSOM) instead of a style attribute, compatible with the existing CSP. The policy is unchanged.
- A row gap keeps translated labels and values with units separated.

`chart.css` generates the upstream Tailwind utilities only within `.shadcn-chart`, without preflight. Colors, font and scale map to Titane tokens. Configs provide labels; the existing stylesheet supplies colors, so `ChartStyle` does not inject a style element.

`OfficialChartTooltip.svelte` connects the selected SVG sample to LayerChart's public manual tooltip state. The context is locked against automatic pointer-leave dismissal: the SVG owns selection and dismissal for mouse, keyboard and touch. A touch selection stays visible until another touch outside, scrolling or dismissal. It uses LayerChart's explicit position props to clamp the measured tooltip within small charts when a side flip alone cannot fit it. The SVG keeps its exact sample selection, collection gaps, boolean steps and Svelte animation.
