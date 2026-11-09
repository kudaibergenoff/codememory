// Renders the table widget. Purely presentational.
export function renderTable(props: { label: string }): string {
  return "<div class=\"table\">" + props.label + "</div>";
}
