import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      "/_pagehub/api": "http://127.0.0.1:8091",
      "/admin/api": "http://127.0.0.1:8091",
    },
  },
});
