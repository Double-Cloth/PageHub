export interface DisplayProject {
  id: string;
  appId: string;
  sourceType?: "app";
  descriptionOverride?: string;
  iconPath?: string;
  displayOrder: number;
  displayEnabled: boolean;
  titleOverride: string;
  keywords: string[];
}
export interface Presentation {
  schemaVersion: number;
  revision: number;
  profile: {
    name: string;
    signature: string;
    avatarPath: string;
    githubUrl: string;
  };
  projects: DisplayProject[];
}
export const defaultPresentation: Presentation = {
  schemaVersion: 1,
  revision: 0,
  profile: {
    name: "Double Cloth",
    signature: "Build. Explore. Create.",
    avatarPath: "/assets/avatar.webp",
    githubUrl: "https://github.com/double-cloth-studio",
  },
  projects: [],
};
