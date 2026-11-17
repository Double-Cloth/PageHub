<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { request } from "../../api/client";
import {
  actionLabels,
  type AppEntry,
  type DeploymentPlan,
  type MaintenanceAction,
} from "../../models/app";

const props = defineProps<{
  app: AppEntry;
  action: MaintenanceAction;
  token: string;
}>();
const emit = defineEmits<{
  close: [];
  submit: [
    approval: {
      password: string;
      confirm_id: string;
      approved_plan_sha256?: string;
      backup?: string;
    },
  ];
}>();
const plan = ref<DeploymentPlan>();
const panel = ref<HTMLElement>();
const updating = props.action === "update" || props.action === "update-self";
const loading = ref(updating);
const error = ref("");
const password = ref("");
const confirmation = ref("");
const backup = ref("");
const controller = new AbortController();
const ready = computed(
  () =>
    !loading.value &&
    !error.value &&
    password.value.length > 0 &&
    confirmation.value === props.app.id &&
    (!updating || !!plan.value) &&
    (props.action !== "restore" || backup.value.length > 0),
);

async function loadPlan() {
  loading.value = true;
  error.value = "";
  try {
    plan.value = await request<DeploymentPlan>(
      `/admin/api/apps/${encodeURIComponent(props.app.id)}/plan`,
      props.token,
      "GET",
      controller.signal,
    );
  } catch (cause) {
    if (!controller.signal.aborted)
      error.value =
        cause instanceof Error ? cause.message : "计划读取失败，请重试";
  } finally {
    loading.value = false;
  }
}
function submit() {
  if (!ready.value) return;
  emit("submit", {
    password: password.value,
    confirm_id: confirmation.value,
    approved_plan_sha256: plan.value?.sha256,
    backup: backup.value || undefined,
  });
  password.value = "";
}
onMounted(() => {
  panel.value?.scrollIntoView({ block: "start" });
  panel.value?.focus({ preventScroll: true });
  if (updating) void loadPlan();
});
onUnmounted(() => {
  controller.abort();
  password.value = "";
});
</script>

<template>
  <section
    ref="panel"
    tabindex="-1"
    class="audit-panel maintenance-panel"
    aria-labelledby="maintenance-title"
  >
    <div class="panel-heading">
      <h2 id="maintenance-title">{{ actionLabels[action] }} {{ app.name }}</h2>
      <button class="text-button" @click="emit('close')">取消操作</button>
    </div>
    <p v-if="action === 'backup' && app.kind === 'compose'">
      备份期间日记会短暂停写，原生校验通过后恢复原运行状态。
    </p>
    <p v-if="action === 'backup' && app.kind === 'static'">
      备份当前静态发布文件，并校验归档的完整性与恢复结果。
    </p>
    <p v-if="action === 'restore'">
      恢复会替换日记、密码和回收站，执行前会备份当前数据。
    </p>
    <p v-if="action === 'rollback'">
      切换到上一发行版本。日记会先备份当前数据，数据格式变更需要核对备份恢复记录。
    </p>
    <p v-if="action === 'uninstall'">
      移除应用容器、公开入口与登记，保留日记数据卷和备份。
    </p>
    <p v-if="loading" role="status">正在核对远程 commit 与当前登记…</p>
    <div v-if="error" role="alert">
      <p>{{ error }}</p>
      <button class="button" @click="loadPlan">重试读取计划</button>
    </div>
    <dl
      v-for="change in plan?.changes.filter((item) => item.id === app.id)"
      :key="change.id"
      class="maintenance-plan"
    >
      <dt>预计动作</dt>
      <dd>{{ change.action }}</dd>
      <dt>当前 commit</dt>
      <dd>
        <code>{{ change.current_commit || "首次部署" }}</code>
      </dd>
      <dt>目标 commit</dt>
      <dd>
        <code>{{ change.target_commit }}</code>
      </dd>
    </dl>
    <form @submit.prevent="submit">
      <label v-if="action === 'restore'"
        >已验证备份目录<input v-model="backup" required autocomplete="off"
      /></label>
      <label
        >输入应用 ID {{ app.id }} 确认作用范围<input
          v-model="confirmation"
          required
          autocomplete="off"
          spellcheck="false"
      /></label>
      <label
        >重新输入管理员密码<input
          v-model="password"
          type="password"
          required
          autocomplete="current-password"
      /></label>
      <button class="button" type="submit" :disabled="!ready">
        确认{{ actionLabels[action] }}
      </button>
    </form>
  </section>
</template>
