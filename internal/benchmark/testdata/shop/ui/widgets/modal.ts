// Renders the modal widget. Purely presentational.
export function renderModal(props: { label: string }): string {
  return "<div class=\"modal\">" + props.label + "</div>";
}
