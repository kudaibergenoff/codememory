// Renders the tooltip widget. Purely presentational.
export function renderTooltip(props: { label: string }): string {
  return "<div class=\"tooltip\">" + props.label + "</div>";
}
