import { useMemo } from "react";
import qrcode from "qrcode-generator";

// A QR code of text, drawn as an SVG: dark modules on white with the four
// modules of quiet zone around them that scanners need, in either theme.
const QUIET_ZONE = 4;

export function QRCode({ text, label, className }: { text: string; label: string; className?: string }) {
  const { size, modules } = useMemo(() => {
    const code = qrcode(0, "M"); // the smallest version that fits, medium error correction
    code.addData(text);
    code.make();
    const count = code.getModuleCount();
    let modules = "";
    for (let row = 0; row < count; row++) {
      for (let col = 0; col < count; col++) {
        if (code.isDark(row, col)) modules += `M${col + QUIET_ZONE} ${row + QUIET_ZONE}h1v1h-1z`;
      }
    }
    return { size: count + 2 * QUIET_ZONE, modules };
  }, [text]);

  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${size} ${size}`} shapeRendering="crispEdges" className={className}>
      <rect width={size} height={size} className="fill-qr-light" />
      <path d={modules} className="fill-qr-dark" />
    </svg>
  );
}
