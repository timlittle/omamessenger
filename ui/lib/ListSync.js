.pragma library

// List synchronization: convert one list of ids to another with minimal
// operations that QML ListModel can apply in order.

// planSync returns an array of operations to transform oldIds to newIds.
// Each op is {op:"remove",index} or {op:"insert",index,id} or {op:"move",from,to}.
// Applying them in order to oldIds must yield newIds.
function planSync(oldIds, newIds) {
  const result = [];
  const current = oldIds.slice();
  const oldSet = new Set(oldIds);
  const newSet = new Set(newIds);

  // First pass: remove ids that are not in newIds.
  for (let i = current.length - 1; i >= 0; i--) {
    if (!newSet.has(current[i])) {
      result.push({ op: 'remove', index: i });
      current.splice(i, 1);
    }
  }

  // Second pass: insert ids that are in newIds but not yet in current.
  // Process newIds from left to right, inserting into current at the right
  // position as we encounter each new id.
  let currentPos = 0;
  for (let newPos = 0; newPos < newIds.length; newPos++) {
    const id = newIds[newPos];

    if (!oldSet.has(id)) {
      // This is a new id; insert it at currentPos.
      result.push({ op: 'insert', index: currentPos, id });
      current.splice(currentPos, 0, id);
      currentPos++;
    } else {
      // This id is already in current; advance currentPos to its position.
      currentPos = current.indexOf(id) + 1;
    }
  }

  // Third pass: move any ids that are out of order.
  for (let newPos = 0; newPos < newIds.length; newPos++) {
    const id = newIds[newPos];
    const oldPos = current.indexOf(id);

    if (oldPos !== newPos) {
      result.push({ op: 'move', from: oldPos, to: newPos });
      current.splice(oldPos, 1);
      current.splice(newPos, 0, id);
    }
  }

  return result;
}

// apply returns a new array after applying ops to ids.
function apply(ids, ops) {
  const result = ids.slice();

  for (const op of ops) {
    if (op.op === 'remove') {
      result.splice(op.index, 1);
    } else if (op.op === 'insert') {
      result.splice(op.index, 0, op.id);
    } else if (op.op === 'move') {
      const item = result.splice(op.from, 1)[0];
      result.splice(op.to, 0, item);
    }
  }

  return result;
}

// upsertById returns a new array: replaces the element with the same id,
// or inserts the item if not found, then keeps the array sorted by compare.
// compare is a function(a, b) that returns <0, 0, or >0. Does not mutate input.
function upsertById(list, item, compare) {
  const result = list.slice();
  const index = result.findIndex(el => el.id === item.id);

  if (index >= 0) {
    result[index] = item;
  } else {
    result.push(item);
  }

  result.sort(compare);
  return result;
}
