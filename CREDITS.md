# Credits and Attribution

BMFT uses third-party resources and libraries. We are grateful to their authors.

## Profanity Dictionary

The profanity filter module uses word forms derived from the **russian-bad-words** project:

- **Repository**: [denexapp/russian-bad-words](https://github.com/denexapp/russian-bad-words)
- **Author**: Denis Mukhametov
- **License**: MIT License
- **Year**: 2020

### MIT License Text

```
MIT License

Copyright (c) 2020 Denis Mukhametov

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### Usage Details

We extracted word forms from the original TypeScript project, compressed them into a binary format (`internal/profanity/dictionary.dat.gz`), and embedded them into BMFT. The dictionary is automatically loaded into the database on first startup (configurable via `PROFANITY_DICT_SOURCE` environment variable).

---

## Other Dependencies

All Go module dependencies are listed in `go.mod` and respect their respective licenses.
