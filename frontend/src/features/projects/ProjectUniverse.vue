<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { select } from "d3-selection";
import { zoom, zoomIdentity, type D3ZoomEvent } from "d3-zoom";
import { gsap } from "gsap";
import type { AppEntry } from "../../models/app";
import { stateLabels } from "../../models/app";
import Icon from "../../ui/Icon.vue";
import { projectLayout } from "./project-layout";

const props = defineProps<{
  apps: AppEntry[];
  sources: {
    x: number;
    y: number;
    width: number;
    height: number;
    color: string;
  }[];
}>();
const emit = defineEmits<{ close: []; prepare: [count: number] }>();
const root = ref<HTMLElement>();
const plane = ref<HTMLElement>();
const query = ref("");
const selected = ref("");
const scale = ref(1);
const width = ref(window.innerWidth);
const height = ref(window.innerHeight);
const layout = computed(() =>
  projectLayout(props.apps.length, width.value, height.value),
);
const matches = computed(() =>
  props.apps.filter((app) =>
    `${app.name} ${app.description} ${app.path} ${app.keywords?.join(" ") ?? ""}`
      .toLowerCase()
      .includes(query.value.trim().toLowerCase()),
  ),
);
const camera = { x: 0, y: 0, k: 1 };
let observer: ResizeObserver;
let animation: gsap.core.Timeline | undefined;
let dragged = false;
let start = { x: 0, y: 0 };
let startTransform = { x: 0, y: 0, k: 1 };
let opening = true;
const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
const behavior = zoom<HTMLElement, unknown>()
  .scaleExtent([0.6, 2.2])
  .clickDistance(8)
  .filter(
    (event) =>
      !event.target.closest("button,input,.search-results") &&
      (!event.button || event.type === "wheel"),
  )
  .on("start", (event: D3ZoomEvent<HTMLElement, unknown>) => {
    if (event.sourceEvent) gsap.killTweensOf(camera);
    dragged = false;
    start = {
      x: event.sourceEvent?.clientX ?? 0,
      y: event.sourceEvent?.clientY ?? 0,
    };
    startTransform = { ...camera };
  })
  .on("zoom", (event: D3ZoomEvent<HTMLElement, unknown>) => {
    const position = event.transform;
    if (
      Math.abs(position.x - startTransform.x) +
        Math.abs(position.y - startTransform.y) >
        8 ||
      Math.abs(position.k - startTransform.k) > 0.01
    )
      dragged = true;
    if (
      event.sourceEvent?.clientX != null &&
      Math.hypot(
        event.sourceEvent.clientX - start.x,
        event.sourceEvent.clientY - start.y,
      ) > 8
    )
      dragged = true;
    Object.assign(camera, { x: position.x, y: position.y, k: position.k });
    draw();
  });
function draw() {
  scale.value = camera.k;
  if (plane.value)
    gsap.set(plane.value, { x: camera.x, y: camera.y, scale: camera.k });
}
function move(x: number, y: number, k = camera.k) {
  gsap.to(camera, {
    x,
    y,
    k,
    duration: reduced ? 0 : 0.55,
    ease: "power3.out",
    onUpdate: () => {
      if (root.value)
        select(root.value).call(
          behavior.transform,
          zoomIdentity.translate(camera.x, camera.y).scale(camera.k),
        );
    },
  });
}
function reset() {
  selected.value = "";
  move(0, 0, 1);
}
function changeScale(delta: number) {
  const next = Math.max(0.6, Math.min(2.2, camera.k + delta));
  const centerX = width.value / 2;
  const centerY = layout.value.centerY;
  move(
    centerX + ((camera.x - centerX) * next) / camera.k,
    centerY + ((camera.y - centerY) * next) / camera.k,
    next,
  );
}
function locate(app: AppEntry) {
  const index = props.apps.findIndex((item) => item.id === app.id);
  const position = layout.value.points[index];
  if (!position) return;
  selected.value = app.id;
  move(-position.x, -position.y, 1);
  query.value = "";
}
function visit(event: MouseEvent) {
  if (dragged || opening) event.preventDefault();
}
function close() {
  emit("close");
}
async function leave() {
  opening = true;
  animation?.kill();
  gsap.killTweensOf(camera);
  if (!root.value) return;
  select(root.value).call(behavior.transform, zoomIdentity);
  await nextTick();
  const tiles = Array.from(
    root.value.querySelectorAll<HTMLElement>(".project-tile"),
  ).slice(0, layout.value.initialCount);
  await new Promise<void>((resolve) => {
    animation = gsap.timeline({ onComplete: resolve });
    tiles.forEach((tile, index) => {
      const source = props.sources[index];
      const rect = tile.getBoundingClientRect();
      animation!.to(
        tile.querySelectorAll(".tile-content"),
        { opacity: 0, duration: reduced ? 0 : 0.12 },
        0,
      );
      animation!.to(
        tile,
        {
          x: source ? source.x - rect.x : 0,
          y: source ? source.y - rect.y : 0,
          scaleX: source ? source.width / rect.width : 0.1,
          scaleY: source ? source.height / rect.height : 0.1,
          backgroundColor: source?.color,
          borderColor: source?.color,
          opacity: 1,
          duration: reduced ? 0 : 0.5,
          ease: "power3.inOut",
        },
        index * 0.02,
      );
    });
    if (!tiles.length) resolve();
  });
}
defineExpose({ leave });
watch(
  () => props.apps.length,
  () => {
    if (
      props.apps.length &&
      !props.apps.some((app) => app.id === selected.value)
    )
      selected.value = "";
  },
);
onMounted(async () => {
  select(root.value!).call(behavior);
  observer = new ResizeObserver((entries) => {
    const bounds = entries[0]!.contentRect;
    const changed =
      width.value !== bounds.width || height.value !== bounds.height;
    width.value = bounds.width;
    height.value = bounds.height;
    if (changed) emit("prepare", layout.value.initialCount);
    const boundX =
      Math.max(
        width.value,
        ...layout.value.points.map((point) => Math.abs(point.x)),
      ) +
      width.value / 2;
    const boundY =
      Math.max(
        height.value,
        ...layout.value.points.map((point) => Math.abs(point.y)),
      ) +
      height.value / 2;
    behavior.translateExtent([
      [-boundX, -boundY],
      [boundX + width.value, boundY + height.value],
    ]);
  });
  observer.observe(root.value!);
  const bounds = root.value!.getBoundingClientRect();
  width.value = bounds.width;
  height.value = bounds.height;
  emit("prepare", layout.value.initialCount);
  await nextTick();
  const tiles = root.value!.querySelectorAll<HTMLElement>(".project-tile");
  animation = gsap.timeline({
    onComplete: () => {
      opening = false;
    },
  });
  Array.from(tiles)
    .slice(0, layout.value.initialCount)
    .forEach((tile, index) => {
      const source = props.sources[index];
      const rect = tile.getBoundingClientRect();
      animation!.from(
        tile,
        {
          x: source ? source.x - rect.x : 0,
          y: source ? source.y - rect.y : 0,
          scaleX: source ? source.width / rect.width : 0.1,
          scaleY: source ? source.height / rect.height : 0.1,
          backgroundColor: source?.color,
          borderColor: source?.color,
          opacity: 1,
          duration: reduced ? 0 : 0.7,
          ease: "power3.inOut",
        },
        index * 0.045,
      );
      animation!.from(
        tile.querySelectorAll(".tile-content"),
        { opacity: 0, duration: reduced ? 0 : 0.25 },
        index * 0.045 + 0.4,
      );
    });
  if (!tiles.length) opening = false;
});
onUnmounted(() => {
  observer?.disconnect();
  animation?.kill();
  gsap.killTweensOf(camera);
  if (root.value) select(root.value).on(".zoom", null);
});
</script>
<template>
  <section ref="root" class="project-universe" aria-label="项目空间">
    <header
      class="universe-header"
      @pointerdown.stop
      @touchstart.stop
      @wheel.stop
    >
      <button class="text-button back-button" @click="close">
        <Icon name="back" /> <span>返回首页</span>
      </button>
      <div class="universe-title">
        项目空间
        <span class="mono">{{ String(apps.length).padStart(2, "0") }}</span>
      </div>
      <div class="project-search">
        <label class="search"
          ><Icon name="search" :size="17" /><input
            v-model="query"
            type="search"
            placeholder="搜索全部项目"
            aria-label="搜索全部项目"
        /></label>
        <div v-if="query.trim()" class="search-results">
          <button v-for="app in matches" :key="app.id" @click="locate(app)">
            <span>{{ app.name }}</span><Icon name="arrow" :size="15" />
          </button>
          <p v-if="!matches.length">没有匹配的项目</p>
        </div>
      </div>
    </header>
    <div ref="plane" class="project-plane">
      <a
        v-for="(app, index) in apps"
        :key="app.id"
        class="project-tile"
        :class="{ selected: selected === app.id }"
        :href="app.path"
        :style="{
          width: `${layout.size}px`,
          height: `${layout.size}px`,
          left: `${width / 2 + layout.points[index]!.x - layout.size / 2}px`,
          top: `${layout.centerY + layout.points[index]!.y - layout.size / 2}px`,
        }"
        @click="visit"
        @dragstart.prevent
      >
        <div class="tile-content tile-top">
          <span class="tile-icon"
            ><img
              v-if="app.iconPath"
              :src="app.iconPath"
              width="28"
              height="28"
              alt=""
              @error="
                ($event.target as HTMLImageElement).style.display = 'none'
              " /><Icon
              v-else
              :name="app.kind === 'static' ? 'code' : 'grid'"
              :size="25" /></span
          ><Icon name="external" :size="14" />
        </div>
        <div class="tile-content tile-bottom">
          <h2>{{ app.name }}</h2>
          <p class="status" :data-state="app.state">
            <i></i>{{ stateLabels[app.state] }}
          </p>
          <p class="tile-description">{{ app.description || app.path }}</p>
        </div>
      </a>
    </div>
    <div v-if="!apps.length" class="universe-empty">
      <Icon name="grid" :size="32" />
      <h2>暂未发布项目</h2>
      <p>已登记且通过离线验证的项目将在这里展示。</p>
      <button class="button" @click="close">返回首页</button>
    </div>
    <div class="universe-bottom" @pointerdown.stop @touchstart.stop @wheel.stop>
      <span class="pan-hint"
        >拖动探索<span v-if="apps.length > layout.initialCount">
          · 还有 {{ apps.length - layout.initialCount }} 个项目在周围</span
        ></span
      >
      <div class="zoom-controls">
        <button aria-label="缩小" @click="changeScale(-0.2)">
          <Icon name="minus" :size="16" /></button
        ><button class="mono" aria-label="重置缩放" @click="reset">
          {{ Math.round(scale * 100) }}%</button
        ><button aria-label="放大" @click="changeScale(0.2)">
          <Icon name="plus" :size="16" /></button
        ><span></span
        ><button aria-label="回到中心" @click="reset">
          <Icon name="reset" :size="16" />
        </button>
      </div>
    </div>
  </section>
</template>
