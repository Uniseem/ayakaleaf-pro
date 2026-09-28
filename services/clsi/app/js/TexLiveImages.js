// Which TeX Live an image reference names, apart from where it is pulled from.
//
// A project records the image it compiles with as a full reference, registry
// and all -- ghcr.io/ayaka-notes/texlive-2023:latest. That is a fact about the
// deployment that created the project, not about the project: move the
// deployment to a different registry, a mirror or another organisation, and
// every project made before the move names an image this server does not
// have. All of them are refused, and the site looks broken to everybody with
// an existing project.
//
// So an image is matched by its last path segment -- the name and the tag --
// which is the part that says which TeX Live it is. That is already how the
// compile cache keys it and what the editor is shown.
//
// Matching stops at the tag. texlive-2023:latest and texlive-2023:2024.1 are
// different TeX Live, and compiling a project against one when it asked for
// the other is worse than refusing: it is exactly the silent change of
// toolchain that makes a document that used to build stop building.

/** The part of an image reference that says which TeX Live it is. */
export function imageIdentity(image) {
  return String(image).split('/').pop()
}

/**
 * The reference this server has for the image that was asked for.
 *
 * Returns what was asked for when there is no allow list to check against, or
 * when nothing matches -- an unknown image stays unknown, and is refused
 * further along with its own name in the message rather than being quietly
 * replaced.
 *
 * @param {string} requested
 * @param {string[]} [allowedImages]
 * @return {string}
 */
export function resolveImage(requested, allowedImages) {
  if (!requested || !allowedImages || allowedImages.length === 0) {
    return requested
  }
  if (allowedImages.includes(requested)) {
    return requested
  }
  const wanted = imageIdentity(requested)
  return allowedImages.find(allowed => imageIdentity(allowed) === wanted) || requested
}

export default { imageIdentity, resolveImage }
