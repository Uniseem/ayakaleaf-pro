/** A span of document positions, closed at both ends. */
export class Range {
  constructor(
    public readonly from: number,
    public readonly to: number
  ) {}

  get length() {
    return this.to - this.from
  }

  contains(pos: number) {
    return pos >= this.from && pos <= this.to
  }

  containsRange(range: Range) {
    return this.from <= range.from && this.to >= range.to
  }

  intersects(range: Range) {
    return this.from < range.to && this.to > range.from
  }

  touchesOrIntersects(range: Range) {
    return this.from <= range.to && this.to >= range.from
  }
}
