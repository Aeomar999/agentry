import { FileQuestion } from "lucide-react";
import { useBreadcrumb } from "@/lib/breadcrumb";
import { EmptyState } from "@/components/molecules/EmptyState";

export function NotFoundPage() {
  useBreadcrumb({ label: "Not found" });
  return <EmptyState icon={<FileQuestion />} title="There's no page here" />;
}
