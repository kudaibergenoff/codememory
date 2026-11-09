// Renders the button widget. Purely presentational.
export function renderButton(props: { label: string }): string {
  return "<div class=\"button\">" + props.label + "</div>";
}
