export function projectLayout(count: number, width: number, height: number) {
  const columns = width < 600 && height > width ? 2 : 3;
  const rows = Math.ceil(Math.min(count, 6) / columns);
  const size = Math.max(
    66,
    Math.min(
      208,
      (width - 64) / columns - 18,
      (height - 188) / Math.max(rows, 1) - 18,
    ),
  );
  const pitch = size + Math.max(12, Math.min(22, size * 0.1));
  const points: { x: number; y: number }[] = [];
  for (let i = 0; i < Math.min(count, 6); i++) {
    const items = Math.min(columns, count - Math.floor(i / columns) * columns);
    points.push({
      x: ((i % columns) - (items - 1) / 2) * pitch,
      y: (Math.floor(i / columns) - (rows - 1) / 2) * pitch,
    });
  }
  // 外围按环扩展，初始六项始终位于中心，追加项目不扰动已有顺序。
  let ring = 1;
  while (points.length < count) {
    const radiusX = width / 2 + pitch * (ring - 0.2);
    const radiusY = Math.max(height / 2, pitch * 2) + pitch * (ring - 0.2);
    for (let side = 0; side < 4 && points.length < count; side++) {
      for (let step = -ring; step <= ring && points.length < count; step++) {
        points.push(
          side % 2 === 0
            ? { x: side === 0 ? radiusX : -radiusX, y: step * pitch }
            : { x: step * pitch, y: side === 1 ? radiusY : -radiusY },
        );
      }
    }
    ring++;
  }
  return { size, points };
}
