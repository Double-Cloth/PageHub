import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { uiFixtures } from "./dev/ui-fixtures";

export default defineConfig(({ mode }) => ({
  plugins: [vue(), ...(mode === "ui" ? [uiFixtures()] : [])],
  server: {
    proxy: mode === "ui" ? undefined : {
      "/_pagehub/api": "http://127.0.0.1:8091",
      "/admin/api": "http://127.0.0.1:8091",
    },
  },
}));
