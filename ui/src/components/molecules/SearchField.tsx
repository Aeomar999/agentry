import { forwardRef } from "react";
import { Search } from "lucide-react";
import { Input } from "@/components/atoms/Input";
import { Kbd } from "@/components/atoms/Kbd";

interface SearchFieldProps {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  shortcut?: string;
}

export const SearchField = forwardRef<HTMLInputElement, SearchFieldProps>(function SearchField(
  { value, onChange, placeholder, shortcut },
  ref,
) {
  return (
    <Input
      ref={ref}
      type="search"
      icon={<Search />}
      value={value}
      placeholder={placeholder}
      onChange={(event) => onChange(event.target.value)}
      onKeyDown={(event) => event.key === "Escape" && event.currentTarget.blur()}
      trailing={shortcut && !value ? <Kbd>{shortcut}</Kbd> : undefined}
      className="w-64 max-w-full"
    />
  );
});
