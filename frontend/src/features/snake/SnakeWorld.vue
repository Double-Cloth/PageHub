<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import { gsap } from "gsap";
import { SnakeEngine, type Direction } from "./engine";

const props = defineProps<{ active: boolean }>();
const emit = defineEmits<{
  change: [mode: string, score: number, speed: number];
  resize: [];
}>();
const host = ref<HTMLDivElement>();
let engine: SnakeEngine;
let renderer: import("pixi.js").Application | undefined;
let graphics: import("pixi.js").Graphics | undefined;
let grid: import("pixi.js").Graphics | undefined;
let disposed = false;
let resizeTimer: ReturnType<typeof setTimeout>;
let last = 0;
let accumulator = 0;
let cell = 22;
let observer: ResizeObserver;
let colorPhase = 0;
const colors = { tile: "#171d22", snake: "#2b363f" };

function report() {
  emit("change", engine.mode, engine.score, engine.speed);
}
function resize() {
  if (!host.value) return;
  const width = host.value.clientWidth;
  const height = host.value.clientHeight;
  if (width < 1 || height < 1) return;
  const columns = Math.max(
    4,
    Math.round(Math.sqrt((width * 3200) / height) / 2) * 2,
  );
  cell = width / columns;
  const rows = Math.max(4, Math.floor(height / cell));
  if (!engine || engine.columns !== columns || engine.rows !== rows) {
    const manual = engine && engine.mode !== "auto";
    colorPhase = engine?.colorPhase ?? colorPhase;
    engine = new SnakeEngine(columns, rows);
    engine.colorPhase = colorPhase;
    if (manual) {
      engine.mode = "paused";
      emit("resize");
    }
    report();
  }
  renderer?.renderer.resize(width, height);
  drawGrid();
  draw();
}
function drawGrid() {
  if (!grid || !renderer) return;
  grid.clear();
  const gap = Math.max(1.3, cell * 0.085);
  for (let y = 0; y < Math.ceil(renderer.screen.height / cell); y++) {
    for (let x = 0; x < engine.columns; x++)
      grid.rect(x * cell, y * cell, cell - gap, cell - gap);
  }
  grid.fill(colors.tile);
}
function draw() {
  if (!graphics || !engine) return;
  graphics.clear();
  const gap = Math.max(1.3, cell * 0.085);
  for (const point of engine.cells)
    graphics.rect(point.x * cell, point.y * cell, cell - gap, cell - gap);
  graphics.fill(colors.snake);
  const head = engine.cells[0]!;
  graphics
    .rect(head.x * cell + 1, head.y * cell + 1, cell - gap - 2, cell - gap - 2)
    .stroke({ color: 0x6e7b83, width: 1 });
  if (engine.food)
    graphics
      .rect(engine.food.x * cell, engine.food.y * cell, cell - gap, cell - gap)
      .fill(0xf04444);
}
function clock() {
  const now = performance.now();
  const elapsed = Math.min(now - last, 180);
  last = now;
  if (!engine || !props.active || document.hidden) {
    accumulator = 0;
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
function sources(count: number) {
  if (!engine) return [];
  const occupied = new Set(
    engine.cells.map((point) => `${point.x},${point.y}`),
  );
  if (engine.food) occupied.add(`${engine.food.x},${engine.food.y}`);
  const available = engine.route.filter(
    (point) => !occupied.has(`${point.x},${point.y}`),
  );
  return Array.from({ length: Math.min(count, available.length) }, () => {
    const point = available.splice(
      Math.floor(Math.random() * available.length),
      1,
    )[0]!;
    return { x: point.x * cell, y: point.y * cell, size: cell - 2 };
  });
}
defineExpose({ start, turn, speed, pause, reset, sources });
watch(
  () => props.active,
  () => {
    last = performance.now();
    accumulator = 0;
  },
);
onMounted(async () => {
  observer = new ResizeObserver(() => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(resize, 140);
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
  clearTimeout(resizeTimer);
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
