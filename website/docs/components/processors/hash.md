---
title: hash
type: processor
---

<!--
     THIS FILE IS AUTOGENERATED!

     To make changes please edit the contents of:
     lib/processor/hash.go
-->


Hashes messages according to the selected algorithm.


import Tabs from '@theme/Tabs';

<Tabs defaultValue="common" values={[
  { label: 'Common', value: 'common', },
  { label: 'Advanced', value: 'advanced', },
]}>

import TabItem from '@theme/TabItem';

<TabItem value="common">

```yaml
# Common config fields, showing default values
hash:
  algorithm: sha256
```

</TabItem>
<TabItem value="advanced">

```yaml
# All config fields, showing default values
hash:
  algorithm: sha256
  parts: []
```

</TabItem>
</Tabs>

This processor is mostly useful when combined with the
[`process_field`](/docs/components/processors/process_field) processor as it allows you to hash a
specific field of a document like this:

``` yaml
# Hash the contents of 'foo.bar'
process_field:
  path: foo.bar
  processors:
  - hash:
      algorithm: sha256
```

## Fields

### `algorithm`

The hash algorithm to use.


Type: `string`  
Default: `"sha256"`  
Options: `sha256`, `sha512`, `sha1`, `xxhash64`.

### `parts`

An optional array of message indexes of a batch that the processor should apply to.
If left empty all messages are processed. This field is only applicable when
batching messages [at the input level](/docs/configuration/batching).

Indexes can be negative, and if so the part will be selected from the end
counting backwards starting from -1.


Type: `array`  
Default: `[]`  

