import type { LayoutDirection } from "../lib/layout";

interface AutoLayoutControlsProps {
  direction: LayoutDirection;
  disabled: boolean;
  onDirectionChange: (direction: LayoutDirection) => void;
  onRun: () => void;
}

export function AutoLayoutControls({
  direction,
  disabled,
  onDirectionChange,
  onRun,
}: AutoLayoutControlsProps) {
  return (
    <div className="auto-layout-controls">
      <select
        aria-label="布局方向"
        onChange={(event) =>
          onDirectionChange(event.target.value as LayoutDirection)
        }
        value={direction}
      >
        <option value="LR">从左到右</option>
        <option value="TB">从上到下</option>
      </select>
      <button disabled={disabled} onClick={onRun} type="button">
        自动整理
      </button>
    </div>
  );
}
