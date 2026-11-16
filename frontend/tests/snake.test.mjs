import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import ts from "typescript";

async function loadModule(path) {
  const source = await readFile(new URL(path, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022,
    },
  });
  return import(
    `data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`
  );
}
const { SnakeEngine, hamiltonian } = await loadModule(
  "../src/features/snake/engine.ts",
);
const { projectLayout } = await loadModule(
  "../src/features/projects/project-layout.ts",
);

test("闭合回路覆盖偶数列棋盘，每一步均相邻", () => {
  for (const [columns, rows] of [
    [4, 4],
    [6, 7],
    [40, 25],
    [64, 33],
  ]) {
    const route = hamiltonian(columns, rows);
    assert.equal(route.length, columns * rows);
    assert.equal(
      new Set(route.map((cell) => `${cell.x},${cell.y}`)).size,
      route.length,
    );
    route.forEach((cell, index) => {
      const next = route[(index + 1) % route.length];
      assert.equal(Math.abs(cell.x - next.x) + Math.abs(cell.y - next.y), 1);
    });
  }
});
test("多尺寸与随机种子下自动蛇持续生存、反色与回收", () => {
  for (const [columns, rows] of [
    [4, 4],
    [5, 7],
    [10, 8],
    [40, 25],
  ]) {
    for (let seed = 1; seed <= 6; seed++) {
      let value = seed;
      const random = () => {
        value = (value * 16807) % 2147483647;
        return (value - 1) / 2147483646;
      };
      const engine = new SnakeEngine(columns, rows, random);
      let flipped = false;
      for (let tick = 0; tick < 25000; tick++) {
        engine.tick();
        assert.equal(
          engine.mode,
          "auto",
          `尺寸 ${columns}×${rows}，种子 ${seed}，步数 ${tick}`,
        );
        assert.equal(
          new Set(engine.cells.map((cell) => `${cell.x},${cell.y}`)).size,
          engine.cells.length,
        );
        assert.ok(
          !engine.food ||
            !engine.cells.some(
              (cell) => cell.x === engine.food.x && cell.y === engine.food.y,
            ),
        );
        if (engine.colorPhase) flipped = true;
      }
      assert.ok(engine.score > 0);
      if (columns <= 10) assert.ok(flipped);
    }
  }
});
test("成长阈值触发单次反色，回收后继续增长", () => {
  const engine = new SnakeEngine(20, 20, () => 0);
  const length = engine.maxGrowthLength;
  engine.cells = engine.route
    .slice(0, length)
    .reverse()
    .map((cell) => ({ ...cell }));
  engine.food = { ...engine.route[length] };
  engine.tick();
  assert.equal(engine.colorPhase, 1);
  assert.equal(engine.recycling, true);
  for (let i = 0; i < length * 4 && engine.recycling; i++) engine.tick();
  assert.equal(engine.mode, "auto");
  assert.equal(engine.colorPhase, 1);
  assert.ok(engine.cells.length < length);
  assert.equal(engine.recycling, false);
});
test("手动游戏禁止掉头、同一步双转，正确计算合法尾格、撞墙与胜利", () => {
  const engine = new SnakeEngine(4, 4, () => 0);
  engine.cells = [
    { x: 1, y: 1 },
    { x: 0, y: 1 },
    { x: 0, y: 2 },
  ];
  engine.direction = "right";
  engine.mode = "manual";
  engine.food = { x: 3, y: 3 };
  engine.turn("left");
  assert.equal(engine.direction, "right");
  engine.turn("up");
  engine.turn("left");
  assert.equal(engine.direction, "up");
  engine.tick();
  engine.tick();
  assert.equal(engine.mode, "dead");
  engine.reset("manual");
  engine.cells = [
    { x: 1, y: 1 },
    { x: 1, y: 2 },
    { x: 0, y: 2 },
    { x: 0, y: 1 },
  ];
  engine.direction = "left";
  engine.food = { x: 3, y: 3 };
  engine.tick();
  assert.equal(engine.mode, "manual");
  engine.reset("manual");
  const route = engine.route;
  engine.cells = route.slice(1).reverse();
  engine.direction = "left";
  engine.food = route[0];
  engine.tick();
  assert.equal(engine.mode, "won");
  assert.equal(engine.score, 1);
});
test("0/1/5/6/7/20/100 个项目具有确定坐标和首六项布局", () => {
  for (const [width, height] of [
    [320, 497],
    [390, 773],
    [844, 331],
    [1440, 822],
    [3840, 2082],
  ]) {
    for (const count of [0, 1, 5, 6, 7, 20, 100]) {
      const first = projectLayout(count, width, height);
      assert.deepEqual(first, projectLayout(count, width, height));
      assert.equal(first.points.length, count);
      assert.equal(
        new Set(first.points.map((point) => `${point.x},${point.y}`)).size,
        count,
      );
      first.points.slice(0, 6).forEach((point) => {
        assert.ok(Math.abs(point.x) + first.size / 2 < width / 2);
        assert.ok(Math.abs(point.y) + first.size / 2 < height / 2);
      });
      first.points
        .slice(6)
        .forEach((point) =>
          assert.ok(
            Math.abs(point.x) > width / 2 || Math.abs(point.y) > height / 2,
          ),
        );
    }
  }
});
