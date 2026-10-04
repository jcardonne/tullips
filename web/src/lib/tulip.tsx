export function Tulip({
  size = 24,
  ...props
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 48 56"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      {...props}
    >
      <path
        d="M24 34v19M24 48C13 48 8 42 7 34c9 0 15 5 17 14Z"
        stroke="#839875"
      />
      <path d="M24 43c2-9 8-13 17-14-1 9-7 14-17 14Z" stroke="#839875" />
      <path
        d="M9 7 18 14 24 3 30 14 39 7v12c0 9-6 15-15 15S9 28 9 19V7Z"
        fill="currentColor"
        fillOpacity=".2"
      />
      <path d="M9 7c15 7 15 14 15 27M39 7c-15 7-15 14-15 27" />
    </svg>
  );
}
