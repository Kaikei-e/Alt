export const normalizeMethod = (value: string | null | undefined): string => {
  if (!value) return "";
  const [head] = value.split(":");
  return head;
};

export const formDataToSubmission = (
  formData: FormData,
): Record<string, string> => {
  const result: Record<string, string> = {};
  formData.forEach((value, key) => {
    if (typeof value === "string") {
      result[key] = value;
      return;
    }

    // File inputs are not currently expected in our auth flows.
    // Preserve the file name to avoid dropping the key entirely.
    if (value instanceof File) {
      result[key] = value.name;
    }
  });

  return result;
};
