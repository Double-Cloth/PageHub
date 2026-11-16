<script setup lang="ts">
import { ref, toRaw } from "vue";
import type { AppEntry } from "../../models/app";
import type { DisplayProject, Presentation } from "../../models/presentation";
import { ApiError, request } from "../../api/client";
import Icon from "../../ui/Icon.vue";
const props = defineProps<{
  settings: Presentation;
  apps: AppEntry[];
  token: string;
}>();
const emit = defineEmits<{
  saved: [settings: Presentation];
  reload: [settings: Presentation];
  close: [];
}>();
const draft = ref<Presentation>(structuredClone(toRaw(props.settings)));
const error = ref("");
const conflict = ref(false);
const saving = ref(false);
const dirty = ref(false);
const dragged = ref(-1);
function move(from: number, to: number) {
  if (to < 0 || to >= draft.value.projects.length) return;
  const project = draft.value.projects.splice(from, 1)[0]!;
  draft.value.projects.splice(to, 0, project);
  dirty.value = true;
}
function add(app: AppEntry) {
  draft.value.projects.push({
    id: app.id,
    appId: app.id,
    sourceType: "app",
    displayOrder: draft.value.projects.length,
    displayEnabled: true,
    titleOverride: "",
    keywords: [],
  });
  dirty.value = true;
}
function remove(project: DisplayProject) {
  if (!confirm("确认移除此主页展示项？应用数据将保留。")) return;
  draft.value.projects = draft.value.projects.filter(
    (item) => item.id !== project.id,
  );
  dirty.value = true;
}
async function save() {
  saving.value = true;
  error.value = "";
  draft.value.projects.forEach((project, index) => {
    project.displayOrder = index;
  });
  try {
    const result = await request<Presentation>(
      "/admin/api/portal",
      props.token,
      "PUT",
      undefined,
      draft.value,
    );
    dirty.value = false;
    emit("saved", result);
  } catch (cause) {
    conflict.value = cause instanceof ApiError && cause.status === 409;
    error.value = cause instanceof Error ? cause.message : "保存失败";
  } finally {
    saving.value = false;
  }
}
async function reload() {
  if (dirty.value && !confirm("重新加载会放弃当前修改，确认继续？")) return;
  try {
    const latest = await request<Presentation>(
      "/admin/api/portal",
      props.token,
    );
    draft.value = structuredClone(latest);
    dirty.value = false;
    conflict.value = false;
    error.value = "";
    emit("reload", latest);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "重新加载失败";
  }
}
function close() {
  if (!dirty.value || confirm("有尚未保存的修改，确认放弃？")) emit("close");
}
function keywords(project: DisplayProject, event: Event) {
  project.keywords = (event.target as HTMLInputElement).value
    .split(/[,，]/)
    .map((word) => word.trim())
    .filter(Boolean);
  dirty.value = true;
}
function undo() {
  draft.value = structuredClone(toRaw(props.settings));
  dirty.value = false;
  error.value = "";
}
function preventLeave(event: BeforeUnloadEvent) {
  if (dirty.value) event.preventDefault();
}
import { onMounted, onUnmounted } from "vue";
onMounted(() => window.addEventListener("beforeunload", preventLeave));
onUnmounted(() => window.removeEventListener("beforeunload", preventLeave));
</script>
<template>
  <section class="manager-panel" aria-label="展示管理" @input="dirty = true">
    <header class="panel-heading">
      <div>
        <h2>展示管理</h2>
        <p>排序决定项目空间的前六个入口。</p>
      </div>
      <button class="icon-button" aria-label="关闭展示管理" @click="close">
        <Icon name="close" />
      </button>
    </header>
    <div class="profile-fields">
      <label
        >显示名称<input v-model="draft.profile.name" maxlength="60" /></label
      ><label
        >签名<input v-model="draft.profile.signature" maxlength="140" /></label
      ><label>本地头像路径<input v-model="draft.profile.avatarPath" /></label>
    </div>
    <ol class="editor-list">
      <li
        v-for="(project, index) in draft.projects"
        :key="project.id"
        @dragover.prevent
        @drop.prevent="move(dragged, index)"
      >
        <button
          class="drag-handle"
          draggable="true"
          aria-label="拖动排序"
          @dragstart="dragged = index"
          @dragend="dragged = -1"
        >
          <Icon name="grid" :size="16" />
        </button>
        <div class="editor-name">
          <input
            v-model="project.titleOverride"
            :placeholder="
              apps.find((app) => app.id === project.appId)?.name ||
              project.appId
            "
            maxlength="80"
          /><span>{{ project.appId }}</span>
          <details class="project-attributes">
            <summary>图标、描述与关键词</summary>
            <label
              >本地图标路径<input
                v-model="project.iconPath"
                placeholder="/assets/projects/icon.webp" /></label
            ><label
              >项目描述<input
                v-model="project.descriptionOverride"
                maxlength="200" /></label
            ><label
              >关键词，以逗号分隔<input
                :value="(project.keywords || []).join(', ')"
                @change="keywords(project, $event)"
            /></label>
          </details>
        </div>
        <label class="checkbox-label"
          ><input
            v-model="project.displayEnabled"
            type="checkbox"
            @change="dirty = true"
          />显示</label
        >
        <button
          class="icon-button"
          :disabled="index === 0"
          aria-label="上移"
          @click="move(index, index - 1)"
        >
          <Icon name="up" :size="16" /></button
        ><button
          class="icon-button"
          :disabled="index === draft.projects.length - 1"
          aria-label="下移"
          @click="move(index, index + 1)"
        >
          <Icon name="down" :size="16" /></button
        ><button class="text-button" @click="remove(project)">移除</button>
      </li>
    </ol>
    <div class="add-projects">
      <h3>添加已登记项目</h3>
      <template v-for="app in apps" :key="app.id"
        ><button
          v-if="!draft.projects.some((project) => project.appId === app.id)"
          class="button"
          :disabled="!app.offlineReady"
          @click="add(app)"
        >
          {{ app.name
          }}<span>{{ app.offlineReady ? "添加" : "待离线验证" }}</span>
        </button></template
      >
      <p>仅可添加公开且通过当前版本离线审核的应用。</p>
    </div>
    <p v-if="error" class="message error" role="alert">
      {{ error
      }}<button v-if="conflict" class="text-button" @click="reload">
        重新加载最新版本
      </button>
    </p>
    <footer class="editor-footer">
      <span class="mono"
        >REV {{ draft.revision }}{{ dirty ? " · 未保存" : "" }}</span
      ><button class="text-button" :disabled="saving || !dirty" @click="undo">
        撤销修改</button
      ><button class="button primary" :disabled="saving" @click="save">
        {{ saving ? "保存中…" : "保存修改" }}<Icon name="check" :size="16" />
      </button>
    </footer>
  </section>
</template>
