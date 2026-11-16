<script setup lang="ts">
defineProps<{ name: string; size?: number }>();
const paths: Record<string, string> = {
  arrow: "M5 12h14m-6-6 6 6-6 6",
  back: "M19 12H5m6-6-6 6 6 6",
  close: "m6 6 12 12M6 18 18 6",
  plus: "M12 5v14M5 12h14",
  minus: "M5 12h14",
  search: "m16 16 4 4M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0",
  grid: "M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z",
  reset: "M4 10a8 8 0 1 1 2 8M4 3v7h7",
  pause: "M8 5v14M16 5v14",
  play: "m8 5 11 7-11 7Z",
  external: "M14 3h7v7m0-7L10 14M10 3H3v18h18v-7",
  message: "M4 4h16v12H9l-5 4Z",
  check: "m5 12 4 4L19 6",
  code: "m8 5-7 7 7 7m8-14 7 7-7 7m-3-17-2 20",
  github:
    "M9 19c-4 1-4-2-6-2m12 4v-3.4c0-1 .1-1.8-.5-2.5 3.1-.4 6.5-1.5 6.5-7a5.5 5.5 0 0 0-1.5-3.8c.2-.7.6-2.4-.2-3.8 0 0-1.3-.4-4.3 1.5a15 15 0 0 0-8 0C4 .1 2.7.5 2.7.5c-.8 1.4-.4 3.1-.2 3.8A5.5 5.5 0 0 0 1 8.1c0 5.5 3.4 6.6 6.5 7-.6.7-.6 1.5-.5 2.5V21",
  up: "m6 14 6-6 6 6",
  down: "m6 10 6 6 6-6",
  lock: "M5 10h14v11H5ZM8 10V6a4 4 0 0 1 8 0v4M12 14v3",
};
</script>
<template>
  <svg
    :width="size || 20"
    :height="size || 20"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="square"
    stroke-linejoin="miter"
    aria-hidden="true"
  >
    <path :d="paths[name] || paths.code" />
  </svg>
</template>
