// Renders the card widget. Purely presentational.
export function renderCard(props: { label: string }): string {
  return "<div class=\"card\">" + props.label + "</div>";
}
