<script setup lang="ts">
import type { Action, AppEntry } from "../../models/app";
import { actionLabels, stateLabels } from "../../models/app";

defineProps<{
  apps: AppEntry[];
  managed: boolean;
  busy: Record<string, boolean>;
}>();
defineEmits<{ action: [app: AppEntry, action: Action] }>();
</script>

<template>
  <ul class="app-list" aria-label="应用列表">
    <li v-for="app in apps" :key="app.id" class="app-row">
      <div class="app-symbol" aria-hidden="true">
        {{ app.name.slice(0, 1).toUpperCase() }}
      </div>
      <div class="app-info">
        <h2>
          <a :href="app.path">{{ app.name }}</a>
        </h2>
        <p v-if="app.description">{{ app.description }}</p>
        <span class="app-path"
          >{{ app.path }}
          <span
            >· {{ app.kind }} ·
            {{ app.startup === "always" ? "常驻应用" : "按需启动" }}</span
          ></span
        >
      </div>
      <span class="status" :data-state="app.state"
        ><i aria-hidden="true"></i
        >{{ stateLabels[app.state] || stateLabels.unknown }}</span
      >
      <div v-if="managed && app.actions?.length" class="app-actions">
        <button
          v-for="action in app.actions"
          :key="action"
          :disabled="busy[app.id]"
          class="button small"
          @click="$emit('action', app, action)"
        >
          {{ actionLabels[action] }}
        </button>
      </div>
      <a class="visit" :href="app.path" :aria-label="`访问 ${app.name}`">
        <span>打开</span
        ><svg
          viewBox="0 0 24 24"
          width="18"
          height="18"
          fill="none"
          stroke="currentColor"
          stroke-width="1.6"
          aria-hidden="true"
        >
          <path d="M5 12h14M13 6l6 6-6 6" />
        </svg>
      </a>
    </li>
  </ul>
</template>
