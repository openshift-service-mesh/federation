#!/bin/bash

if [ -z "$1" ]; then
  echo "Usage: $0 <input_drawio_file>"
  exit 1
fi

INPUT_FILE="$1"

if [ ! -f "${INPUT_FILE}" ]; then
  echo "error: file '${INPUT_FILE}' not found"
  exit 1
fi

PAGE_NAMES=($(grep -oP '<diagram.*?name="\K[^"]+' "${INPUT_FILE}" | tr '[:upper:]' '[:lower:]'))

if [ ${#PAGE_NAMES[@]} -eq 0 ]; then
  echo "error: no pages found in the input file"
  exit 1
fi

# Loop through each page name and export it
for i in "${!PAGE_NAMES[@]}"; do
  OUTPUT_FILE="./${INPUT_FILE%.*}-${PAGE_NAMES[$i]}.png"

  drawio --export --page-index "$i" -f png -o "$OUTPUT_FILE" "$INPUT_FILE"

  if [ $? -ne 0 ]; then
    echo "error: failed exporting page '${PAGE_NAMES[$i]}'"
    exit 1
  fi
done
