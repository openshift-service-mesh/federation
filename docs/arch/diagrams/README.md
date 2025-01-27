# Architectural diagrams

This folder contains diagrams describing architecture of Service Mesh Federation.


## Dev Guide

Each image is generated from the `.drawio` file.

### Prerequisites

#### draw.io editor

We highly recommend using drawio desktop editor or IDE plugin of your choice.

To install desktop editor, choose the appropriate package for your operating system:

- **Fedora** :
```sh
DRAWIO=26.0.7 && \
wget https://github.com/jgraph/drawio-desktop/releases/download/v${DRAWIO}/drawio-x86_64-${DRAWIO}.rpm -O drawio.rpm && \
sudo rpm -i drawio.rpm
```

> [!NOTE]
> Visit the [draw.io desktop GitHub releases page](https://github.com/jgraph/drawio-desktop/releases) for other types of packages.

- **macOS:** Install via Homebrew:
```sh
  brew install --cask drawio
```
### Generation

To generate PNG images from the draw.io file, run the following command:

```shell
./export.sh <your-diagram>.drawio
```

This will export each page in to an individual PNG file.

> [!IMPORTANT]
> Script requires `drawio` desktop editor which can be used in headless mode.
