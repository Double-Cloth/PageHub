<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import { gsap } from "gsap";
import { SnakeEngine, type Direction } from "./engine";

const props = defineProps<{ active: boolean }>();
const emit = defineEmits<{
  change: [mode: string, score: number, speed: number];
  layout: [];
}>();
const host = ref<HTMLDivElement>();
let engine: SnakeEngine;
let renderer: import("pixi.js").Application | undefined;
let graphics: import("pixi.js").Graphics | undefined;
let grid: import("pixi.js").Graphics | undefined;
let disposed = false;
let resizeFrame = 0;
let last = 0;
let accumulator = 0;
let cellWidth = 22;
let cellHeight = 22;
let observer: ResizeObserver;
let colorPhase = 0;
const liftedCells = new Set<string>();
const colors = { tile: "#171d22", snake: "#2b363f" };

function report() {
  emit("change", engine.mode, engine.score, engine.speed);
}
function resize() {
  if (!host.value) return;
  const width = host.value.clientWidth;
  const height = host.value.clientHeight;
  if (width < 1 || height < 1) return;
  let targetCell = Math.sqrt((width * height) / 3200);
  if (engine) {
    const points = engine.food ? [...engine.cells, engine.food] : engine.cells;
    const xs = points.map((point) => point.x);
    const ys = points.map((point) => point.y);
    const spanX = Math.max(...xs) - Math.min(...xs) + 1;
    const spanY = Math.max(...ys) - Math.min(...ys) + 1;
    // 缩窄窗口时等比缩小网格，确保整条蛇仍在合法棋盘内。
    targetCell = Math.min(
      targetCell,
      width / (spanX + (spanX % 2)),
      height / spanY,
    );
  }
  const columns = Math.max(4, Math.floor(width / targetCell / 2) * 2);
  const rows = Math.max(4, Math.floor(height / targetCell));
  if (!engine) engine = new SnakeEngine(columns, rows);
  else if (engine.columns !== columns || engine.rows !== rows)
    engine.resize(columns, rows);
  cellWidth = width / engine.columns;
  cellHeight = height / engine.rows;
  report();
  renderer?.renderer.resize(width, height);
  drawGrid();
  draw();
  renderer?.render();
  emit("layout");
}
function drawGrid() {
  if (!grid || !renderer) return;
  grid.clear();
  const gap = Math.max(0.7, Math.min(cellWidth, cellHeight) * 0.085);
  for (let y = 0; y < engine.rows; y++) {
    for (let x = 0; x < engine.columns; x++) {
      if (liftedCells.has(`${x},${y}`)) continue;
      grid.rect(
        x * cellWidth,
        y * cellHeight,
        cellWidth - gap,
        cellHeight - gap,
      );
    }
  }
  grid.fill(colors.tile);
}
function draw() {
  if (!graphics || !engine) return;
  graphics.clear();
  const gap = Math.max(0.7, Math.min(cellWidth, cellHeight) * 0.085);
  for (const point of engine.cells)
    graphics.rect(
      point.x * cellWidth,
      point.y * cellHeight,
      cellWidth - gap,
      cellHeight - gap,
    );
  graphics.fill(colors.snake);
  const head = engine.cells[0]!;
  graphics
    .rect(
      head.x * cellWidth + 0.5,
      head.y * cellHeight + 0.5,
      cellWidth - gap - 1,
      cellHeight - gap - 1,
    )
    .stroke({ color: 0x6e7b83, width: 1 });
  if (engine.food)
    graphics
      .rect(
        engine.food.x * cellWidth,
        engine.food.y * cellHeight,
        cellWidth - gap,
        cellHeight - gap,
      )
      .fill(0xf04444);
}
function clock() {
  const now = performance.now();
  const elapsed = Math.min(now - last, 180);
  last = now;
  if (!engine || !props.active || document.hidden) {
    return;
  }
  accumulator += elapsed;
  const interval = 1000 / (engine.mode === "auto" ? 12 : engine.speed);
  if (accumulator < interval) return;
  accumulator %= interval;
  engine.tick();
  if (engine.colorPhase !== colorPhase) {
    colorPhase = engine.colorPhase;
    gsap.to(colors, {
      tile: colorPhase ? "#2b363f" : "#171d22",
      snake: colorPhase ? "#171d22" : "#2b363f",
      duration: 0.65,
      onUpdate: () => {
        drawGrid();
        draw();
      },
    });
  }
  draw();
  report();
}
function start(direction: Direction) {
  engine?.manual(direction);
  report();
}
function turn(direction: Direction) {
  engine?.turn(direction);
}
function speed(delta: number) {
  engine.speed = Math.max(5, Math.min(16, engine.speed + delta));
  report();
}
function pause() {
  if (engine.mode !== "manual" && engine.mode !== "paused") return;
  engine.mode = engine.mode === "paused" ? "manual" : "paused";
  report();
}
function reset(manual = false) {
  engine.reset(manual ? "manual" : "auto");
  draw();
  report();
}
function automatic() {
  engine?.automatic();
  report();
}
function sources(count: number) {
  if (!engine) return [];
  const occupied = new Set(
    engine.cells.map((point) => `${point.x},${point.y}`),
  );
  if (engine.food) occupied.add(`${engine.food.x},${engine.food.y}`);
  const rect = host.value!.getBoundingClientRect();
  const projectBounds = document
    .querySelector(".project-universe")
    ?.getBoundingClientRect();
  const visibleBottom = (projectBounds?.bottom ?? rect.bottom) - rect.y;
  const available = engine.route.filter(
    (point) =>
      !occupied.has(`${point.x},${point.y}`) &&
      point.y * cellHeight >= 100 &&
      (point.y + 1) * cellHeight <= visibleBottom - 48,
  );
  const gap = Math.max(0.7, Math.min(cellWidth, cellHeight) * 0.085);
  liftedCells.clear();
  const selected = Array.from(
    { length: Math.min(count, available.length) },
    () => {
      const point = available.splice(
        Math.floor(Math.random() * available.length),
        1,
      )[0]!;
      liftedCells.add(`${point.x},${point.y}`);
      return {
        x: rect.x + point.x * cellWidth,
        y: rect.y + point.y * cellHeight,
        width: cellWidth - gap,
        height: cellHeight - gap,
        color: colors.tile,
      };
    },
  );
  drawGrid();
  renderer?.render();
  return selected;
}

function restoreSources() {
  liftedCells.clear();
  drawGrid();
  renderer?.render();
}

defineExpose({
  start,
  turn,
  speed,
  pause,
  reset,
  automatic,
  sources,
  restoreSources,
});
watch(
  () => props.active,
  () => {
    last = performance.now();
  },
);
onMounted(async () => {
  observer = new ResizeObserver(() => {
    cancelAnimationFrame(resizeFrame);
    resizeFrame = requestAnimationFrame(resize);
  });
  observer.observe(host.value!);
  resize();
  try {
    const { Application, Graphics } = await import("pixi.js");
    if (disposed) return;
    const app = new Application();
    await app.init({
      backgroundAlpha: 0,
      antialias: false,
      resolution: Math.min(devicePixelRatio, 2),
      autoDensity: true,
      autoStart: false,
      preference: "webgl",
    });
    if (disposed) {
      app.destroy(true);
      return;
    }
    renderer = app;
    host.value!.classList.remove("static-grid");
    host.value!.appendChild(app.canvas);
    grid = new Graphics();
    graphics = new Graphics();
    app.stage.addChild(grid, graphics);
    resize();
    // GSAP 统一调度视觉渲染，游戏仿真使用独立固定步长。
    last = performance.now();
    gsap.ticker.add(clock);
    gsap.ticker.add(render);
  } catch {
    host.value?.classList.add("static-grid");
  }
});
function render() {
  if (!document.hidden && props.active) renderer?.render();
}
onUnmounted(() => {
  disposed = true;
  cancelAnimationFrame(resizeFrame);
  observer?.disconnect();
  gsap.ticker.remove(clock);
  gsap.ticker.remove(render);
  gsap.killTweensOf(colors);
  renderer?.destroy(true, { children: true });
});
</script>
<template>
  <div ref="host" class="snake-world static-grid" aria-hidden="true"></div>
</template>
