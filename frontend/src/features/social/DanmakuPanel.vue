<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { ApiError, request } from "../../api/client";
import Icon from "../../ui/Icon.vue";
export interface Message {
  id: string;
  content: string;
  created_at: string;
}
const props = defineProps<{ open: boolean; token: string }>();
const emit = defineEmits<{ close: []; message: [message: Message] }>();
const messages = ref<Message[]>([]);
const content = ref("");
const error = ref("");
const available = ref(false);
const sending = ref(false);
const cooldown = ref(0);
const enabled = ref(true);
const length = computed(() => Array.from(content.value.trim()).length);
let source: EventSource;
let clock: ReturnType<typeof setInterval>;
let disposed = false;
let replay = 0;
let reconnect: ReturnType<typeof setTimeout>;
let retry = 0;
function receive(message: Message, animate = true) {
  if (messages.value.some((item) => item.id === message.id)) return;
  messages.value = [message, ...messages.value].slice(0, 50);
  if (animate) emit("message", message);
}
async function load() {
  const started = Date.now();
  try {
    const data = await request<{ messages: Message[]; enabled: boolean }>(
      "/_pagehub/social/messages",
    );
    if (disposed) return;
    const live = messages.value.filter(
      (message) => new Date(message.created_at).getTime() > started,
    );
    messages.value = data.messages;
    live.forEach((message) => receive(message, false));
    messages.value.sort((a, b) => b.created_at.localeCompare(a.created_at));
    enabled.value = data.enabled;
    available.value = true;
    error.value = "";
  } catch {
    available.value = false;
    error.value = "弹幕服务暂不可用，请稍后重试。";
  }
}
async function send() {
  if (
    sending.value ||
    cooldown.value ||
    !available.value ||
    !enabled.value ||
    !length.value ||
    length.value > 100
  )
    return;
  sending.value = true;
  error.value = "";
  try {
    const message = await request<Message>(
      "/_pagehub/social/messages",
      "",
      "POST",
      undefined,
      { content: content.value.trim() },
    );
    receive(message);
    content.value = "";
    cooldown.value = 15;
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 429)
      cooldown.value = cause.retryAfter || 15;
    error.value = cause instanceof Error ? cause.message : "发送失败";
  } finally {
    sending.value = false;
  }
}
async function remove(message: Message) {
  try {
    await request(
      `/admin/api/social/messages/${message.id}/delete`,
      props.token,
      "POST",
    );
    messages.value = messages.value.filter((item) => item.id !== message.id);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "删除失败";
  }
}
async function toggle() {
  try {
    await request("/admin/api/social/settings", props.token, "PUT", undefined, {
      enabled: !enabled.value,
    });
    enabled.value = !enabled.value;
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "更新失败";
  }
}
function connect() {
  if (disposed) return;
  source = new EventSource("/_pagehub/social/events");
  source.addEventListener("message", (event) => {
    try {
      receive(JSON.parse(event.data));
      available.value = true;
    } catch {
      /* 忽略损坏的推送，历史接口仍可重载。 */
    }
  });
  source.addEventListener("delete", (event) => {
    messages.value = messages.value.filter((item) => item.id !== event.data);
  });
  source.addEventListener("settings", (event) => {
    enabled.value = JSON.parse(event.data).enabled;
  });
  source.onopen = () => {
    retry = 0;
    void load();
  };
  source.onerror = () => {
    source.close();
    clearTimeout(reconnect);
    reconnect = setTimeout(
      connect,
      Math.min(60000, 1000 * 2 ** Math.min(retry++, 6)),
    );
  };
}
onMounted(() => {
  void load();
  connect();
  clock = setInterval(() => {
    if (cooldown.value > 0) cooldown.value--;
    replay++;
    if (replay % 18 === 3 && messages.value.length && !document.hidden)
      emit(
        "message",
        messages.value[Math.floor(replay / 18) % messages.value.length]!,
      );
  }, 1000);
});
onUnmounted(() => {
  disposed = true;
  source?.close();
  clearTimeout(reconnect);
  clearInterval(clock);
});
</script>
<template>
  <aside v-if="open" class="side-panel social-panel" aria-label="弹幕面板">
    <div class="panel-heading">
      <div>
        <h2>留下一个想法</h2>
        <p>在这个小小的数字空间里相遇。</p>
      </div>
      <button class="icon-button" aria-label="关闭弹幕" @click="emit('close')">
        <Icon name="close" />
      </button>
    </div>
    <form @submit.prevent="send">
      <label for="danmaku-input">你的弹幕</label
      ><textarea
        id="danmaku-input"
        v-model="content"
        placeholder="Say something…"
        rows="3"
        :disabled="!available || !enabled"
      />
      <div class="composer-bottom">
        <span class="mono" :class="{ invalid: length > 100 }"
          >{{ length }} / 100</span
        ><button
          class="button primary"
          :disabled="
            sending ||
            cooldown > 0 ||
            !available ||
            !enabled ||
            length < 1 ||
            length > 100
          "
        >
          {{
            cooldown ? `${cooldown}s 后发送` : sending ? "发送中…" : "发送弹幕"
          }}<Icon name="arrow" :size="16" />
        </button>
      </div>
    </form>
    <p v-if="!enabled" class="message">管理员已暂停发布弹幕。</p>
    <p v-if="error" class="message error" role="alert">
      {{ error }} <button class="text-button" @click="load">重试</button>
    </p>
    <div class="history-heading">
      <span>最近的声音</span><span class="mono">{{ messages.length }}</span
      ><button v-if="token" class="text-button" @click="toggle">
        {{ enabled ? "暂停发布" : "恢复发布" }}
      </button>
    </div>
    <ol class="message-history">
      <li v-for="message in messages" :key="message.id">
        <p>{{ message.content }}</p>
        <time>{{
          new Date(message.created_at).toLocaleString("zh-CN")
        }}</time><button
          v-if="token"
          class="text-button"
          @click="remove(message)"
        >
          删除
        </button>
      </li>
    </ol>
    <p v-if="!messages.length && available" class="empty-copy">
      还没有弹幕，写下第一条吧。
    </p>
  </aside>
</template>
