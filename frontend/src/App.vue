<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import AppCatalog from "./features/app-catalog/AppCatalog.vue";
import { loadApps, loadStatus, request, runAction } from "./api/client";
import type { Action, AppEntry, Job } from "./models/app";
import { actionLabels } from "./models/app";

const adminMode = /^\/admin\/?$/.test(window.location.pathname);
const apps = ref<AppEntry[]>([]);
const tokenInput = ref("");
const token = ref("");
const query = ref("");
const kind = ref("all");
const loading = ref(false);
const notice = ref("");
const error = ref("");
const snapshot = ref(false);
const busy = ref<Record<string, boolean>>({});
const lastUpdated = ref("");
let timer: ReturnType<typeof setInterval> | undefined;
let disposed = false;
let generation = 0;

const filtered = computed(() =>
  apps.value.filter(
    (app) =>
      (kind.value === "all" || kind.value === app.kind) &&
      `${app.name} ${app.description || ""} ${app.path}`
        .toLocaleLowerCase()
        .includes(query.value.toLocaleLowerCase()),
  ),
);
const healthy = computed(
  () => apps.value.filter((app) => app.state === "healthy").length,
);

async function refresh(force = false) {
  if (loading.value && !force) return;
  loading.value = true;
  error.value = "";
  const current = ++generation;
  try {
    let entries: AppEntry[];
    let fromSnapshot = false;
    try {
      entries = await loadApps(token.value);
    } catch (cause) {
      if (token.value) throw cause;
      entries = await request<AppEntry[]>("/_pagehub/catalog.json");
      fromSnapshot = true;
    }
    if (disposed || generation !== current) return;
    snapshot.value = fromSnapshot;
    apps.value = entries.map((app) => ({ ...app, state: "unknown" }));
    if (!snapshot.value) {
      await Promise.allSettled(
        apps.value.map(async (app) => {
          const status = await loadStatus(app.id, token.value);
          if (generation === current && !disposed) app.state = status.state;
        }),
      );
    }
    if (generation === current && !disposed)
      lastUpdated.value = new Date().toLocaleTimeString("zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
  } catch (cause) {
    if (generation === current && !disposed)
      error.value = cause instanceof Error ? cause.message : "应用清单加载失败";
  } finally {
    if (generation === current) loading.value = false;
  }
}

async function login() {
  const credential = tokenInput.value.trim();
  if (!credential) return;
  error.value = "";
  try {
    await loadApps(credential);
    token.value = credential;
    tokenInput.value = "";
    await refresh(true);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "连接失败";
  }
}

function logout() {
  token.value = "";
  tokenInput.value = "";
  void refresh(true);
}

async function action(app: AppEntry, operation: Action) {
  if (
    operation !== "start" &&
    !window.confirm(
      `确认${actionLabels[operation]} ${app.name}？当前连接可能中断。`,
    )
  )
    return;
  busy.value[app.id] = true;
  error.value = "";
  notice.value = "";
  try {
    let job = await runAction(app.id, operation, token.value);
    app.state = operation === "stop" ? "stopping" : "starting";
    const deadline = Date.now() + 210_000;
    while (job.state === "queued" || job.state === "running") {
      if (disposed) return;
      if (Date.now() > deadline)
        throw new Error("任务等待超时，请查看任务记录和实际运行状态");
      await new Promise((resolve) => setTimeout(resolve, 1000));
      job = await request<Job>(`/admin/api/jobs/${job.id}`, token.value);
    }
    if (job.state === "error") throw new Error(job.error || "任务执行失败");
    notice.value = `${app.name} ${actionLabels[operation]}任务已完成`;
    await refresh();
  } catch (cause) {
    await refresh();
    error.value = cause instanceof Error ? cause.message : "操作失败";
  } finally {
    delete busy.value[app.id];
  }
}

onMounted(() => {
  void refresh();
  timer = setInterval(() => {
    if (!Object.keys(busy.value).length) void refresh();
  }, 30_000);
});
onUnmounted(() => {
  disposed = true;
  generation++;
  clearInterval(timer);
});
</script>

<template>
  <header class="site-header">
    <a class="brand" href="/" aria-label="PageHub 首页"
      ><span class="brand-mark" aria-hidden="true">P</span>PageHub</a
    >
    <div class="header-right">
      <span>{{ adminMode ? "本机管理" : "应用门户" }}</span
      ><button v-if="token" class="button small" @click="logout">
        退出管理
      </button>
    </div>
  </header>
  <main>
    <section class="intro">
      <div>
        <h1>{{ adminMode ? "管理你的应用" : "所有应用，一个入口。" }}</h1>
        <p>
          {{
            adminMode
              ? "查看运行状态，并管理已登记的应用。"
              : "在这里找到并访问已发布的应用。"
          }}
        </p>
      </div>
      <button class="button" :disabled="loading" @click="refresh()">
        {{ loading ? "正在刷新…" : "刷新状态" }}
      </button>
    </section>
    <form
      v-if="adminMode && !token"
      class="login-panel"
      @submit.prevent="login"
    >
      <div>
        <h2>管理员连接</h2>
        <p>通过 SSH 转发访问本机管理端，凭据仅保存在当前页面内存。</p>
      </div>
      <label for="admin-token">管理员 Token</label>
      <div class="login-controls">
        <input
          id="admin-token"
          v-model="tokenInput"
          type="password"
          autocomplete="off"
          required
          placeholder="输入管理员 Token"
        /><button class="button primary" type="submit">连接</button>
      </div>
    </form>
    <p v-if="snapshot" class="message" role="status">
      正在显示静态应用清单，实时状态暂不可用。
    </p>
    <p v-if="error" class="message error" role="alert">{{ error }}</p>
    <p v-if="notice" class="message success" role="status">{{ notice }}</p>
    <section class="catalog" aria-label="应用目录">
      <div class="catalog-heading">
        <h2>
          应用目录 <span>{{ apps.length }}</span>
        </h2>
        <p>
          {{ healthy }} 个运行正常<span v-if="lastUpdated">
            · {{ lastUpdated }} 更新</span
          >
        </p>
      </div>
      <div class="toolbar">
        <label class="search"
          ><svg
            viewBox="0 0 24 24"
            width="19"
            height="19"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            aria-hidden="true"
          >
            <circle cx="10.5" cy="10.5" r="6.5" />
            <path d="m16 16 4 4" /></svg
          ><input
            v-model="query"
            type="search"
            aria-label="搜索应用"
            placeholder="搜索名称、描述或路径" /></label
        ><select v-model="kind" aria-label="筛选应用类型">
          <option value="all">全部类型</option>
          <option value="static">静态站点</option>
          <option value="image">单镜像应用</option>
          <option value="compose">Compose 应用</option>
          <option value="external-compose">既有 Compose</option>
          <option value="external">外部服务</option>
        </select>
      </div>
      <AppCatalog
        v-if="filtered.length"
        :apps="filtered"
        :managed="!!token"
        :busy="busy"
        @action="action"
      />
      <div v-else class="empty">
        <h3>
          {{
            loading
              ? "正在加载应用…"
              : apps.length
                ? "没有匹配的应用"
                : "还没有公开应用"
          }}
        </h3>
        <p>
          {{
            apps.length
              ? "试试其他名称或应用类型。"
              : "管理员导入并发布应用后，会自动出现在这里。"
          }}
        </p>
      </div>
    </section>
  </main>
  <footer><span>PageHub</span><span>应用独立运行 · 统一入口</span></footer>
</template>
